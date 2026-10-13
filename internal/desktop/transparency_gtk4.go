//go:build linux && !gtk3

package desktop

/*
#cgo pkg-config: gtk4
#include <gtk/gtk.h>
#include <stdlib.h>

static void kt_install_transparent_css(const char *css) {
	GdkDisplay *display = gdk_display_get_default();
	if (display == NULL) {
		return;
	}
	GtkCssProvider *provider = gtk_css_provider_new();
	gtk_css_provider_load_from_string(provider, css);
	gtk_style_context_add_provider_for_display(display, GTK_STYLE_PROVIDER(provider),
		GTK_STYLE_PROVIDER_PRIORITY_USER);
	g_object_unref(provider);
}
*/
import "C"

import "unsafe"

// Wails v3 (GTK4) makes the webview transparent but leaves the theme's window
// background in place, which shows up as a rectangle around the rounded card.
// Clear the background of frameless windows and their container box; the
// webview's own children (e.g. <select> popovers) keep their styling.
const transparentCSS = `
window.wails-frameless,
window.wails-frameless > box,
window.wails-frameless > box > webkitwebview {
	background: none;
	background-color: transparent;
	box-shadow: none;
	border: none;
}
`

// MakeFramelessWindowsTransparent installs the CSS on the default display.
// It must run on the GTK main thread after GTK has been initialised.
func MakeFramelessWindowsTransparent() {
	css := C.CString(transparentCSS)
	defer C.free(unsafe.Pointer(css))
	C.kt_install_transparent_css(css)
}
