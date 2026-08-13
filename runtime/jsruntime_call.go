package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/dop251/goja"
)

// Calling JS from Go.
//
// init.js starts as a window bootstrapper, but it is also a place to put app logic that
// benefits from being editable without a rebuild — formatting, routing rules, per-customer
// tweaks. This is the Go -> JS direction: the script defines functions, Go calls them.
//
// THE CONSTRAINT THAT DICTATES EVERYTHING BELOW: a goja.Runtime is not goroutine-safe.
// Before this file, that was fine by accident — Run() was called exactly once, from the
// startup goroutine, and nothing else ever touched the VM. Bridge handlers run one
// goroutine per request, so the first Call() from a handler would have been a data race.
//
// So the VM gets ONE owning goroutine and callers hand it work over a channel. A mutex is
// the obvious alternative and is wrong: a JS function that calls back into Go which calls
// JS again deadlocks a non-reentrant mutex, and that re-entry is exactly what a scripting
// layer invites. A queue serialises without ever blocking the VM on itself.
//
// Not gated by Policy, deliberately. init.js is app-author code with the same trust level
// as backend/app/app.go, and Go already has unrestricted access to everything Policy
// protects — gating a call Go chose to make would be theatre. The reverse direction
// (JS calling bridge commands) is a different question and DOES need the ACL; see
// docs/roadmap.md Track J.

// ErrJSUnavailable is returned when the JS runtime is not accepting calls — either no init
// script defined any functions, or the runtime has been stopped.
var ErrJSUnavailable = errors.New("goleo: JS runtime unavailable")

// jsInlineKey marks a context as already running on the VM's owning goroutine, so a nested
// Call executes inline instead of deadlocking on the queue. Set only by the JS -> Go
// binding in provideAPI; a caller cannot forge it usefully, since the worst it could do is
// run their own call on their own goroutine.
type jsInlineKey struct{}

// jsJob is one unit of work for the owning goroutine.
type jsJob struct {
	fn    func(vm *goja.Runtime) (any, error)
	reply chan jsResult
}

type jsResult struct {
	val any
	err error
}

// JS returns the app's script runtime, or nil if there is none. Callers should treat a nil
// return the same as a disabled feature rather than panicking — an app with no init.js is
// the normal case, not an error.
func (a *App) JS() *JSRuntime { return a.jsr }

// startLoop claims the VM for a single goroutine. Every subsequent VM access goes through
// jobs; nothing else may touch jsr.vm once this returns.
//
// Called at the end of Run(), so the init script itself still executes inline on the
// startup goroutine — it must, because createWindow is thread-affine and the window has to
// be created on the thread Run() locked.
func (jsr *JSRuntime) startLoop() {
	jsr.loopOnce.Do(func() {
		jsr.jobs = make(chan jsJob)
		jsr.done = make(chan struct{})
		go func() {
			for {
				select {
				case job := <-jsr.jobs:
					val, err := job.fn(jsr.vm)
					job.reply <- jsResult{val: val, err: err}
				case <-jsr.done:
					return
				}
			}
		}()
	})
}

// submit hands fn to the owning goroutine and waits for it, honouring ctx.
//
// A cancelled context interrupts the VM rather than merely abandoning the caller: a
// runaway script (`while(true){}`) would otherwise wedge the owning goroutine for the life
// of the process and every later call with it. goja's Interrupt is the only way out, and
// it needs something to fire it.
func (jsr *JSRuntime) submit(ctx context.Context, fn func(vm *goja.Runtime) (any, error)) (any, error) {
	if jsr == nil || jsr.jobs == nil {
		return nil, ErrJSUnavailable
	}
	// Re-entry: we are already ON the owning goroutine, because a script called into Go
	// (goleo.invoke) and that handler is now calling back into JS. Queueing here would
	// block the goroutine waiting for itself — a deadlock, and the single hazard the
	// one-goroutine design introduces. Running inline is safe for exactly the reason the
	// queue exists: nothing else can be touching the VM while this goroutine holds it.
	if ctx.Value(jsInlineKey{}) != nil {
		return fn(jsr.vm)
	}
	reply := make(chan jsResult, 1)
	select {
	case jsr.jobs <- jsJob{fn: fn, reply: reply}:
	case <-jsr.done:
		return nil, ErrJSUnavailable
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	select {
	case r := <-reply:
		return r.val, r.err
	case <-ctx.Done():
		// The job is already running on the owning goroutine. Interrupt unwinds it with a
		// goja.InterruptedError, which the job's own error path turns into a Go error, so
		// the goroutine survives for the next caller.
		jsr.vm.Interrupt(ctx.Err())
		<-reply
		jsr.vm.ClearInterrupt()
		return nil, ctx.Err()
	case <-jsr.done:
		return nil, ErrJSUnavailable
	}
}

// Call invokes a global function defined by the init script and returns its result.
//
// Arguments and the return value cross as JSON. That is deliberate: goja's reflective
// mapping will happily accept a struct and then silently drop fields it cannot represent,
// which is the same failure the gomobile providers hit (see AGENTS.md — provider methods
// take JSON strings for exactly this reason). A predictable boundary that rejects loudly
// beats a clever one that loses data quietly.
//
// A JS exception becomes a Go error; it never panics across the boundary.
//
//	total, err := app.JS().Call(ctx, "priceOrder", order)
func (jsr *JSRuntime) Call(ctx context.Context, name string, args ...any) (any, error) {
	if name == "" {
		return nil, fmt.Errorf("goleo: JS call needs a function name")
	}
	return jsr.submit(ctx, func(vm *goja.Runtime) (val any, err error) {
		// A goja panic (a JS throw surfacing as *goja.Exception, or an interrupt) must not
		// escape into the caller's goroutine.
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("goleo: JS %s panicked: %v", name, r)
			}
		}()

		raw := vm.Get(name)
		if raw == nil || goja.IsUndefined(raw) || goja.IsNull(raw) {
			return nil, fmt.Errorf("goleo: JS function %q is not defined (is it declared in init.js?)", name)
		}
		fn, ok := goja.AssertFunction(raw)
		if !ok {
			return nil, fmt.Errorf("goleo: JS %q is not a function", name)
		}

		jsArgs := make([]goja.Value, 0, len(args))
		for i, a := range args {
			v, err := toJSValue(vm, a)
			if err != nil {
				return nil, fmt.Errorf("goleo: JS %s arg %d: %w", name, i, err)
			}
			jsArgs = append(jsArgs, v)
		}

		out, err := fn(goja.Undefined(), jsArgs...)
		if err != nil {
			var ex *goja.Exception
			if errors.As(err, &ex) {
				return nil, fmt.Errorf("goleo: JS %s threw: %s", name, ex.Value())
			}
			return nil, fmt.Errorf("goleo: JS %s: %w", name, err)
		}
		if out == nil || goja.IsUndefined(out) || goja.IsNull(out) {
			return nil, nil
		}
		return out.Export(), nil
	})
}

// CallJSON is Call with the result decoded into out, for when the script returns a shape
// the caller already models in Go.
func (jsr *JSRuntime) CallJSON(ctx context.Context, name string, out any, args ...any) error {
	v, err := jsr.Call(ctx, name, args...)
	if err != nil {
		return err
	}
	if v == nil {
		return nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("goleo: JS %s result is not encodable: %w", name, err)
	}
	if err := json.Unmarshal(b, out); err != nil {
		return fmt.Errorf("goleo: JS %s result does not fit %T: %w", name, out, err)
	}
	return nil
}

// Has reports whether the init script defined a callable global with this name, so a caller
// can offer a scripted hook without requiring one.
func (jsr *JSRuntime) Has(ctx context.Context, name string) bool {
	v, err := jsr.submit(ctx, func(vm *goja.Runtime) (any, error) {
		raw := vm.Get(name)
		if raw == nil || goja.IsUndefined(raw) || goja.IsNull(raw) {
			return false, nil
		}
		_, ok := goja.AssertFunction(raw)
		return ok, nil
	})
	if err != nil {
		return false
	}
	b, _ := v.(bool)
	return b
}

// toJSValue routes Go values through JSON so what JS receives is predictable: plain
// objects, arrays, numbers, strings. Primitives skip the round trip because encoding them
// changes nothing and the allocation is pure waste on the common path.
func toJSValue(vm *goja.Runtime, a any) (goja.Value, error) {
	switch a.(type) {
	case nil, bool, string,
		int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64:
		return vm.ToValue(a), nil
	}
	b, err := json.Marshal(a)
	if err != nil {
		return nil, fmt.Errorf("not encodable as JSON: %w", err)
	}
	var generic any
	if err := json.Unmarshal(b, &generic); err != nil {
		return nil, fmt.Errorf("not decodable: %w", err)
	}
	return vm.ToValue(generic), nil
}

// loopFields are embedded in JSRuntime; kept here so the call machinery stays in one file.
type loopFields struct {
	jobs     chan jsJob
	done     chan struct{}
	loopOnce sync.Once
	stopOnce sync.Once
}

// stopLoop releases the owning goroutine. Queued and in-flight callers get
// ErrJSUnavailable rather than blocking forever on a runtime that is going away.
func (jsr *JSRuntime) stopLoop() {
	jsr.stopOnce.Do(func() {
		if jsr.done != nil {
			close(jsr.done)
		}
	})
}

// provideBridgeAPI installs the JS -> Go direction: a `goleo` global whose methods reach
// the same bridge commands the frontend uses.
//
// Routed through Bridge.HandleRequestContext, NOT the handler map, and that is the whole
// security design. HandleRequestContext is where Policy is enforced (bridge.go), so the
// capability check applies here for free and cannot drift out of sync. A second path into
// the handler map is precisely how an ACL gets bypassed later without anyone noticing.
//
// Synchronous on purpose: Go's bridge handlers are synchronous, so invoke returns the
// result directly and the engine needs no event loop or Promise machinery. That does mean
// a slow handler blocks the script — and, while it runs, any Go -> JS call queued behind
// it. For init.js, which is app-author code, that is the right trade; it is not a
// general-purpose async runtime and does not pretend to be.
func (jsr *JSRuntime) provideBridgeAPI() {
	if jsr.app == nil || jsr.app.bridge == nil {
		return // no app (tests, or a runtime constructed standalone)
	}

	obj := jsr.vm.NewObject()

	obj.Set("invoke", func(call goja.FunctionCall) goja.Value {
		method := call.Argument(0).String()
		if method == "" {
			panic(jsr.vm.ToValue("goleo.invoke needs a method name"))
		}

		var args json.RawMessage
		if a := call.Argument(1); !goja.IsUndefined(a) && !goja.IsNull(a) {
			b, err := json.Marshal(a.Export())
			if err != nil {
				panic(jsr.vm.ToValue(fmt.Sprintf("goleo.invoke(%s): arguments are not encodable: %v", method, err)))
			}
			args = b
		}

		// The marker is what lets a handler call back into JS without deadlocking on the
		// goroutine it is already running on. See submit().
		ctx := context.WithValue(context.Background(), jsInlineKey{}, true)
		resp := jsr.app.bridge.HandleRequestContext(ctx, InvokeRequest{Method: method, Args: args})
		if resp.Error != "" {
			// Surfaces in JS as a throw, so a script can try/catch it like any other error
			// — including "permission denied" from the Policy check.
			panic(jsr.vm.ToValue(resp.Error))
		}
		return jsr.vm.ToValue(resp.Result)
	})

	obj.Set("emit", func(call goja.FunctionCall) goja.Value {
		event := call.Argument(0).String()
		if event == "" {
			panic(jsr.vm.ToValue("goleo.emit needs an event name"))
		}
		var payload any
		if p := call.Argument(1); !goja.IsUndefined(p) && !goja.IsNull(p) {
			payload = p.Export()
		}
		jsr.app.Emit(event, payload)
		return goja.Undefined()
	})

	jsr.vm.Set("goleo", obj)
}
