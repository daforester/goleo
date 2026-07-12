//go:build linux && !android && cgo

package camera

/*
#include <stdlib.h>
#include <string.h>
#include <errno.h>
#include <fcntl.h>
#include <unistd.h>
#include <sys/ioctl.h>
#include <sys/mman.h>
#include <sys/select.h>
#include <sys/time.h>
#include <linux/videodev2.h>

// goleo_v4l2_capture grabs a single MJPEG frame from a V4L2 device (a USB/
// built-in webcam at /dev/videoN). MJPEG is requested so the dequeued buffer
// is already a JPEG image — no colour conversion needed. All V4L2 structs are
// handled here in C so cgo never has to model the kernel unions/ABI.
//
// On success it returns 0, sets *out to a malloc'd JPEG buffer (caller frees
// with free()) and *outLen to its length. On failure it returns non-zero and
// points *errmsg at a static description.
static int goleo_v4l2_capture(const char *dev, int want_w, int want_h,
                              unsigned char **out, int *outLen,
                              const char **errmsg) {
    int fd = open(dev, O_RDWR | O_NONBLOCK);
    if (fd < 0) { *errmsg = "cannot open device"; return -1; }

    struct v4l2_format fmt;
    memset(&fmt, 0, sizeof(fmt));
    fmt.type = V4L2_BUF_TYPE_VIDEO_CAPTURE;
    fmt.fmt.pix.width = want_w > 0 ? want_w : 640;
    fmt.fmt.pix.height = want_h > 0 ? want_h : 480;
    fmt.fmt.pix.pixelformat = V4L2_PIX_FMT_MJPEG;
    fmt.fmt.pix.field = V4L2_FIELD_ANY;
    if (ioctl(fd, VIDIOC_S_FMT, &fmt) < 0) { *errmsg = "VIDIOC_S_FMT failed"; close(fd); return -2; }
    if (fmt.fmt.pix.pixelformat != V4L2_PIX_FMT_MJPEG) {
        *errmsg = "device does not support MJPEG capture";
        close(fd); return -3;
    }

    struct v4l2_requestbuffers req;
    memset(&req, 0, sizeof(req));
    req.count = 1;
    req.type = V4L2_BUF_TYPE_VIDEO_CAPTURE;
    req.memory = V4L2_MEMORY_MMAP;
    if (ioctl(fd, VIDIOC_REQBUFS, &req) < 0 || req.count < 1) {
        *errmsg = "VIDIOC_REQBUFS failed"; close(fd); return -4;
    }

    struct v4l2_buffer buf;
    memset(&buf, 0, sizeof(buf));
    buf.type = V4L2_BUF_TYPE_VIDEO_CAPTURE;
    buf.memory = V4L2_MEMORY_MMAP;
    buf.index = 0;
    if (ioctl(fd, VIDIOC_QUERYBUF, &buf) < 0) { *errmsg = "VIDIOC_QUERYBUF failed"; close(fd); return -5; }

    void *mem = mmap(NULL, buf.length, PROT_READ | PROT_WRITE, MAP_SHARED, fd, buf.m.offset);
    if (mem == MAP_FAILED) { *errmsg = "mmap failed"; close(fd); return -6; }

    if (ioctl(fd, VIDIOC_QBUF, &buf) < 0) { *errmsg = "VIDIOC_QBUF failed"; munmap(mem, buf.length); close(fd); return -7; }

    enum v4l2_buf_type type = V4L2_BUF_TYPE_VIDEO_CAPTURE;
    if (ioctl(fd, VIDIOC_STREAMON, &type) < 0) { *errmsg = "VIDIOC_STREAMON failed"; munmap(mem, buf.length); close(fd); return -8; }

    int rc = -9;
    unsigned char *result = NULL;
    int resultLen = 0;
    // Grab a handful of frames and keep one, so auto-exposure/white-balance
    // have a moment to settle rather than returning a black first frame.
    for (int attempt = 0; attempt < 20; attempt++) {
        fd_set fds; FD_ZERO(&fds); FD_SET(fd, &fds);
        struct timeval tv; tv.tv_sec = 2; tv.tv_usec = 0;
        int r = select(fd + 1, &fds, NULL, NULL, &tv);
        if (r < 0) { if (errno == EINTR) continue; *errmsg = "select failed"; rc = -9; break; }
        if (r == 0) { *errmsg = "timed out waiting for a frame"; rc = -9; break; }

        struct v4l2_buffer dq;
        memset(&dq, 0, sizeof(dq));
        dq.type = V4L2_BUF_TYPE_VIDEO_CAPTURE;
        dq.memory = V4L2_MEMORY_MMAP;
        if (ioctl(fd, VIDIOC_DQBUF, &dq) < 0) {
            if (errno == EAGAIN) continue;
            *errmsg = "VIDIOC_DQBUF failed"; rc = -10; break;
        }
        if (attempt < 4) { ioctl(fd, VIDIOC_QBUF, &dq); continue; } // warm-up frames
        result = (unsigned char *)malloc(dq.bytesused);
        if (!result) { *errmsg = "out of memory"; ioctl(fd, VIDIOC_QBUF, &dq); rc = -11; break; }
        memcpy(result, mem, dq.bytesused);
        resultLen = (int)dq.bytesused;
        ioctl(fd, VIDIOC_QBUF, &dq);
        rc = 0;
        break;
    }

    ioctl(fd, VIDIOC_STREAMOFF, &type);
    munmap(mem, buf.length);
    close(fd);

    if (rc == 0) { *out = result; *outLen = resultLen; }
    return rc;
}
*/
import "C"

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"unsafe"
)

// platformCapturePhoto captures one JPEG frame from a local V4L2 camera. If no
// device is present or capture fails, it returns an error and the JS bridge
// falls back to the webview's getUserMedia (see bridge/src/camera.ts).
func platformCapturePhoto() (*PhotoData, error) {
	dev, err := findVideoDevice()
	if err != nil {
		return nil, err
	}

	cDev := C.CString(dev)
	defer C.free(unsafe.Pointer(cDev))

	var out *C.uchar
	var outLen C.int
	var cErr *C.char
	rc := C.goleo_v4l2_capture(cDev, 640, 480, &out, &outLen, &cErr)
	if rc != 0 {
		msg := "unknown error"
		if cErr != nil {
			msg = C.GoString(cErr)
		}
		return nil, fmt.Errorf("camera: V4L2 capture on %s failed: %s", dev, msg)
	}
	defer C.free(unsafe.Pointer(out))

	// PhotoData.Data is []byte, which marshals to base64 JSON — exactly what the
	// frontend expects for a "jpeg" photo.
	return &PhotoData{Data: C.GoBytes(unsafe.Pointer(out), outLen), Format: "jpeg"}, nil
}

func findVideoDevice() (string, error) {
	if _, err := os.Stat("/dev/video0"); err == nil {
		return "/dev/video0", nil
	}
	if matches, _ := filepath.Glob("/dev/video*"); len(matches) > 0 {
		return matches[0], nil
	}
	return "", fmt.Errorf("camera: %w (no /dev/video* device found)", errors.ErrUnsupported)
}

func platformStartStream(map[string]any) error {
	return fmt.Errorf("camera: %w (continuous streaming; use capturePhoto)", errors.ErrUnsupported)
}

func platformStopStream() error {
	return nil
}
