# NSGo patches

These patches apply, in order, to the NSGo release named in `go.mod` (`github.com/nsgo-dev/nsgo v0.3.4`):

1. `0001`: a screen another covers keeps its layout, so its texts, list rows and scroll offsets are there as it shows again.
2. `0002`: Android's predictive back. A back handler can name the app's own layers a back gesture moves (`core.BackOptions.Preview`, `Previewed`): the page shrinks, rounds its corners and leans away from the gesture's edge as Android's own pages do, over the layer it uncovers, whose scrim fades out. `BackOptions.Takes` says when a handler would leave back alone; when nothing would take back, the view hands it to the system, which previews leaving the app.

`0002` changes the Android runtime's Java, so after applying the patches rebuild `classes.dex` and `classes.jar` with a JDK 17+ and an Android SDK with platform 36 and build-tools 36+ (CI does this through `rebuild-android-runtime`):

```sh
git clone --depth 1 --branch v0.3.4 https://github.com/nsgo-dev/nsgo.git nsgo
git -C nsgo am "$PWD"/app-nsgo/patches/nsgo/*.patch
ANDROID_HOME=~/Library/Android/sdk sh nsgo/internal/platform/android/runtime/build.sh
```

`go.mod` replaces the module with `../nsgo`, so the checkout goes beside `app-nsgo`. Drop a patch once upstream merges it, and the replace once none is left.
