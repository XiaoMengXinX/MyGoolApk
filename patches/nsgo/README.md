# NSGo patches

These patches apply, in order, to the NSGo release named in `go.mod` (`github.com/nsgo-dev/nsgo v0.3.3`):

1. `0001`: a zoomed scroll view stays still as fingers land and lift on Android (upstream PR #2301).
2. `0002`: a pinch zooms a scroll view out until the fingers meet on Android (upstream PR #2311).
3. `0003`: a screen another covers keeps its layout, so its texts, list rows and scroll offsets are there as it shows again.

The Android patches change only the runtime's Java. Rebuild `classes.dex` and `classes.jar` after applying them, with a JDK 17+ and an Android SDK that holds platform 36 and d8:

```sh
git clone --depth 1 --branch v0.3.3 https://github.com/nsgo-dev/nsgo.git nsgo
git -C nsgo am "$PWD"/app-nsgo/patches/nsgo/*.patch
ANDROID_HOME=/path/to/sdk sh nsgo/internal/platform/android/runtime/build.sh
```

`go.mod` replaces the module with `../nsgo`, so the checkout goes beside `app-nsgo`. Drop a patch once upstream merges it, and the replace once none is left.
