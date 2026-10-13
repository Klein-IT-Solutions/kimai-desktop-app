package hotkey

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"testing"
	"time"
)

// Needs an X server (e.g. Xvfb) and xdotool; skipped otherwise.
func TestX11GrabAndPress(t *testing.T) {
	if os.Getenv("DISPLAY") == "" {
		t.Skip("no X display")
	}
	if _, err := exec.LookPath("xdotool"); err != nil {
		t.Skip("xdotool not installed")
	}
	acc, _ := Parse("Super+Shift+F9")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pressed := make(chan struct{}, 4)
	if err := listenX11(ctx, acc, func() { pressed <- struct{}{} }, nil); err != nil {
		t.Fatal(err)
	}

	// A second grab of the same shortcut must report it as taken.
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	if err := listenX11(ctx2, acc, func() {}, nil); !errors.Is(err, errKeyTaken) {
		t.Errorf("second grab: err = %v, want errKeyTaken", err)
	}

	for _, keys := range []string{"super+shift+F9", "super+shift+F9"} {
		if out, err := exec.Command("xdotool", "key", keys).CombinedOutput(); err != nil {
			t.Fatalf("xdotool: %v %s", err, out)
		}
		select {
		case <-pressed:
		case <-time.After(2 * time.Second):
			t.Fatal("shortcut press not received")
		}
	}

	// With Num Lock on the shortcut must still work.
	exec.Command("xdotool", "key", "Num_Lock").Run()
	defer exec.Command("xdotool", "key", "Num_Lock").Run()
	exec.Command("xdotool", "key", "super+shift+F9").Run()
	select {
	case <-pressed:
	case <-time.After(2 * time.Second):
		t.Error("shortcut not received with Num Lock on")
	}

	cancel()
	time.Sleep(200 * time.Millisecond)
	// After release, the key can be grabbed again.
	ctx3, cancel3 := context.WithCancel(context.Background())
	defer cancel3()
	if err := listenX11(ctx3, acc, func() {}, nil); err != nil {
		t.Errorf("re-grab after release: %v", err)
	}
}

// A held key (with X auto-repeat) must yield exactly one press and one release.
func TestX11PushToTalk(t *testing.T) {
	if os.Getenv("DISPLAY") == "" {
		t.Skip("no X display")
	}
	if _, err := exec.LookPath("xdotool"); err != nil {
		t.Skip("xdotool not installed")
	}
	acc, _ := Parse("Super+Shift+F8")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := make(chan string, 64)
	if err := listenX11(ctx, acc, func() { events <- "press" }, func() { events <- "release" }); err != nil {
		t.Fatal(err)
	}
	// Simulate auto-repeat: release+press pairs while the key is held.
	cmd := "keydown super+shift+F8 sleep 0.3"
	for i := 0; i < 5; i++ {
		cmd += " keyup F8 keydown F8 sleep 0.05"
	}
	cmd += " sleep 0.3 keyup F8 keyup super+shift"
	if out, err := exec.Command("sh", "-c", "xdotool "+cmd).CombinedOutput(); err != nil {
		t.Fatalf("xdotool: %v %s", err, out)
	}
	time.Sleep(300 * time.Millisecond)
	close(events)
	var got []string
	for e := range events {
		got = append(got, e)
	}
	if len(got) != 2 || got[0] != "press" || got[1] != "release" {
		t.Errorf("events = %v, want [press release]", got)
	}
}
