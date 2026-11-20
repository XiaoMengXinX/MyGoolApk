package app

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/nsgo-dev/nsgo/core"
)

// imageStore keeps pictures on disk. Coolapk's CDN answers only requests
// that carry the app's user agent, which the system's image loader does not
// send, so the store downloads them and the views show the files.
type imageStore struct {
	dir       string
	userAgent string
	client    *http.Client
	slots     chan struct{}

	mu       sync.Mutex
	inflight map[string]*download
}

type download struct {
	done chan struct{}
	path string
	err  error
}

func newImageStore(dir, userAgent string) *imageStore {
	return &imageStore{
		dir:       dir,
		userAgent: userAgent,
		client:    &http.Client{Timeout: 30 * time.Second},
		slots:     make(chan struct{}, 6),
		inflight:  make(map[string]*download),
	}
}

func (s *imageStore) pathOf(url string) string {
	sum := sha256.Sum256([]byte(url))
	return filepath.Join(s.dir, hex.EncodeToString(sum[:16]))
}

// cached returns the file of url when it was downloaded before.
func (s *imageStore) cached(url string) (string, bool) {
	p := s.pathOf(url)
	if _, err := os.Stat(p); err == nil {
		return p, true
	}
	return "", false
}

// fetch returns the file of url, downloading it unless it is on disk. Calls
// for one url share a download.
func (s *imageStore) fetch(url string) (string, error) {
	if p, ok := s.cached(url); ok {
		return p, nil
	}
	s.mu.Lock()
	d := s.inflight[url]
	if d == nil {
		d = &download{done: make(chan struct{})}
		s.inflight[url] = d
		go func() {
			d.path, d.err = s.download(url)
			s.mu.Lock()
			delete(s.inflight, url)
			s.mu.Unlock()
			close(d.done)
		}()
	}
	s.mu.Unlock()
	<-d.done
	return d.path, d.err
}

func (s *imageStore) download(url string) (string, error) {
	s.slots <- struct{}{}
	defer func() { <-s.slots }()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", s.userAgent)
	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("image %s: %s", url, resp.Status)
	}
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(s.dir, "dl-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())
	if _, err := io.Copy(tmp, resp.Body); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	p := s.pathOf(url)
	return p, os.Rename(tmp.Name(), p)
}

// pic shows the picture at url, which may change as a list row moves to
// another item. Until it is on disk the view shows the placeholder color
// its class names.
func (a *App) pic(cx *core.Scope, url func() string, fit core.ResizeMode, class string, style core.Style) core.Node {
	src := core.NewSignal(cx, "")
	cx.Effect(func() {
		u := url()
		if u == "" {
			src.Set("")
			return
		}
		if p, ok := a.images.cached(u); ok {
			src.Set(p)
			return
		}
		src.Set("")
		go func() {
			p, err := a.images.fetch(u)
			if err != nil {
				return
			}
			cx.Post(func() {
				if url() == u {
					src.Set(p)
				}
			})
		}()
	})
	return core.Image{
		Class:      class,
		Style:      style,
		ResizeMode: fit,
		SourceFunc: func() core.ImageSource { return core.Src(src.Get()) },
	}
}

// picOf is pic for a url that never changes.
func (a *App) picOf(cx *core.Scope, url string, fit core.ResizeMode, class string, style core.Style) core.Node {
	return a.pic(cx, func() string { return url }, fit, class, style)
}
