package flavor

// Edition identity — the ONE file that differs between the product branches.
//
// `main` is the union: every flavor, secretary by default. The `student` and
// `business-secretary` branches override only this file, so the entire shared
// engine stays byte-identical across editions and improvements cross-port
// without conflicts. If you are about to change behaviour per product, it
// almost certainly belongs here and nowhere else.

// EditionName labels the build, for `brain mode` and the about box.
const EditionName = "full"

// Offered is which personas this edition exposes, in display order. The app's
// flavor pills and `brain mode` read this directly.
var Offered = []Flavor{Secretary, Tutor, Business}

// Default is the persona a fresh install starts in.
var Default = Secretary
