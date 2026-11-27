# NSGo patches

These patches apply, in order, to the NSGo release named in `go.mod` (`github.com/nsgo-dev/nsgo v0.3.4`):

1. `0001`: a screen another covers keeps its layout, so its texts, list rows and scroll offsets are there as it shows again.
2. `0002`: Android's predictive back. A back handler can name the app's own layers a back gesture moves (`core.BackOptions.Preview`, `Previewed`): the page shrinks, rounds its corners and leans away from the gesture's edge as Android's own pages do, over the layer it uncovers, whose scrim fades out. `BackOptions.Takes` says when a handler would leave back alone; when nothing would take back, the view hands it to the system, which previews leaving the app.

3. `0003`: `core.View.ScrollTarget` hands the scrolls that start on a view to a scroll view it names: on macOS the wheel and touchpad over it, on Android a drag along the scroll view's axis, natively and with the platform's glide. A head laid over lists, as on the topic and event pages, scrolls the list shown through it. Windows, Linux and the web hand nothing yet.
4. `0004`: on iOS, screens that slide in, out or under a swipe back run at the screen's full rate. Without it a ProMotion iPhone shows them at 60 Hz unless something else in the app animates.
5. `0005`: iOS's `ScrollTarget`. A vertical drag on the view moves the target's content natively, stretching past its edges and gliding on at UIKit's normal deceleration, at the screen's full rate.
6. `0006`: zoomable scroll content follows its children's bounds, so centering a smaller image leaves the margins outside its scrollable area. iOS centers the content again after viewport changes. Android releases a cancelled drag without starting a fling.
7. `0007`: `mobile.SavePhoto` adds an encoded image to Photos on iOS with add-only authorization, or to Android's MediaStore. Android 28 requests storage permission; newer Android versions need none. The callback reports the library write's completion.

`0002`, `0003`, `0006` and `0007` change the Android runtime's Java, so after applying the patches rebuild `classes.dex` and `classes.jar` with a JDK 17+ and an Android SDK with platform 36 and build-tools 36+ (CI does this through `rebuild-android-runtime`):

```sh
git clone --depth 1 --branch v0.3.4 https://github.com/nsgo-dev/nsgo.git nsgo
git -C nsgo am "$PWD"/app-nsgo/patches/nsgo/*.patch
ANDROID_HOME=~/Library/Android/sdk sh nsgo/internal/platform/android/runtime/build.sh
```

`go.mod` replaces the module with `../nsgo`, so the checkout goes beside `app-nsgo`. Drop a patch once upstream merges it, and the replace once none is left.

Local development can select a separate patched framework worktree with a Go workspace. Keep that workspace file local.
