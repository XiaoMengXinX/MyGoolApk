//go:build ios

package app

import (
	"fmt"

	"github.com/ebitengine/purego/objc"
)

func systemVersion() string {
	info := objc.ID(objc.GetClass("NSProcessInfo")).Send(objc.RegisterName("processInfo"))
	type version struct{ Major, Minor, Patch int }
	v := objc.Send[version](info, objc.RegisterName("operatingSystemVersion"))
	if v.Patch != 0 {
		return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
	}
	return fmt.Sprintf("%d.%d", v.Major, v.Minor)
}
