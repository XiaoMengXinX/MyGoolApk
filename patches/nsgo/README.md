# NSGo patches

These patches apply, in order, to the NSGo release named in `go.mod` (`github.com/nsgo-dev/nsgo v0.3.4`):

1. `0001`: a screen another covers keeps its layout, so its texts, list rows and scroll offsets are there as it shows again.

This patch changes Go code only, so no Android runtime rebuild is needed:

```sh
git clone --depth 1 --branch v0.3.4 https://github.com/nsgo-dev/nsgo.git nsgo
git -C nsgo am "$PWD"/app-nsgo/patches/nsgo/*.patch
```

`go.mod` replaces the module with `../nsgo`, so the checkout goes beside `app-nsgo`. Drop the patch and the replace once upstream merges it.
