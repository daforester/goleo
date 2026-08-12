# Vendored Gradle wrapper

`gradle-wrapper.jar` is committed here rather than downloaded at build time.

**Provenance:** `https://github.com/gradle/gradle/raw/v9.4.1/gradle/wrapper/gradle-wrapper.jar`
— the canonical source, at the tag matching `gradleWrapperVersion` in `cli/cmd/build.go`.
Apache License 2.0, like Gradle itself.

- size: 48966 bytes
- sha256: `55243ef57851f12b070ad14f7f5bb8302daceeebc5bce5ece5fa6edb23e1145c`

**Why vendored.** `goleo build android` previously fetched this over the network with
`http.Get` — no timeout, so a hung connection hung the build indefinitely, and no integrity
check whatsoever on a JAR that is then executed via `java -classpath`. Anyone able to
interpose on that request could run code on a developer's machine and inside their CI.

A pinned checksum was the alternative, but that means owning a checksum treadmill for every
Gradle bump. Vendoring removes the network from the build path entirely and matches what the
repo already does for Go dependencies (see AGENTS.md → Vendoring): third-party code is
committed so builds never break and never depend on an upstream still being reachable.

**Updating.** Change `gradleWrapperVersion` in `cli/cmd/build.go` *and*
`distributionUrl` in `cli/cmd/templates/android/gradle/wrapper/gradle-wrapper.properties`
(a test asserts they agree), then replace this JAR from the matching tag and update the
size and hash above. Verify before committing that it is a real wrapper JAR:

```bash
unzip -l gradle-wrapper.jar | grep GradleWrapperMain
```
