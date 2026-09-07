# Vendored Logos standard library (v0.4.9, MIT)

Source: https://github.com/codetesla51/logos/tree/v0.4.9/std

`use "std/..."` in scripts resolves against these files: kyfram passes
them to the interpreter as its std FS (the raw `interpreter` package has
no embedded copy of its own — only the `lgs` CLI binary does).

Refresh on Logos upgrade: copy `std/*.lgs` from the new version and bump
the version line above.
