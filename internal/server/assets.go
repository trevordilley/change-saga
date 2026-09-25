package server

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"net/http"
)

// The reviewer's navigation is htmx: one vendored file with no dependencies
// and no build step, served from this binary like app.js. See
// assets/htmx/VERSION for where each file came from and docs/design/
// htmx-frontend.md for why.
//
//go:embed assets/htmx/htmx.min.js assets/htmx/preload.min.js
var vendoredScripts embed.FS

// shellAsset is one file every page of a session shares. Its URL names a
// digest of its bytes, so a browser keeps it for as long as it likes and a
// new build is a new URL rather than a stale copy.
type shellAsset struct {
	name        string
	contentType string
	body        []byte
	hash        string
}

// Path is where the page links the asset.
func (asset *shellAsset) Path() string { return "/assets/" + asset.hash + "/" + asset.name }

// appStyles is the whole reviewer stylesheet. It used to be inlined into
// every page; as a file it is parsed once and cached for good.
const appStyles = pageStyles + reviewStyles + technicalStyles + technicalERDStyles

var shellAssets = func() map[string]*shellAsset {
	vendored := func(name string) []byte {
		data, err := vendoredScripts.ReadFile("assets/htmx/" + name)
		if err != nil {
			panic(err)
		}
		return data
	}
	assets := map[string]*shellAsset{}
	for _, asset := range []*shellAsset{
		{name: "app.css", contentType: "text/css; charset=utf-8", body: []byte(appStyles)},
		{name: "theme.js", contentType: "text/javascript; charset=utf-8", body: []byte(themeBoot)},
		{name: "htmx.min.js", contentType: "text/javascript; charset=utf-8", body: vendored("htmx.min.js")},
		{name: "preload.min.js", contentType: "text/javascript; charset=utf-8", body: vendored("preload.min.js")},
		{name: "app.js", contentType: "text/javascript; charset=utf-8", body: []byte(appJavaScript)},
	} {
		digest := sha256.Sum256(asset.body)
		asset.hash = hex.EncodeToString(digest[:])[:16]
		assets[asset.name] = asset
	}
	return assets
}()

// assetPath is the versioned URL of a shell asset, for the templates.
func assetPath(name string) string {
	asset, ok := shellAssets[name]
	if !ok {
		panic("unknown shell asset " + name)
	}
	return asset.Path()
}

// shellAssetFile serves a versioned asset. Only the current digest answers:
// an old URL is a 404 rather than new bytes under a name a browser may keep
// for a year.
func (a *app) shellAssetFile(w http.ResponseWriter, r *http.Request) {
	asset, ok := shellAssets[r.PathValue("name")]
	if !ok || asset.hash != r.PathValue("hash") {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", asset.contentType)
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	_, _ = w.Write(asset.body)
}

// htmxConfig is read by htmx from the page's meta tag. The page's CSP allows
// no eval and no inline script, so neither is asked for; htmx's indicator
// styles are not injected; and no page is ever snapshotted into storage: a
// page is large, and a snapshot would be Saga content older than the request
// that restores it. Back and forward ask the server instead.
const htmxConfig = `{"allowEval":false,"allowScriptTags":false,"includeIndicatorStyles":false,"historyCacheSize":0,"refreshOnHistoryMiss":false,"defaultSettleDelay":0,"scrollIntoViewOnBoost":false,"selfRequestsOnly":true}`
