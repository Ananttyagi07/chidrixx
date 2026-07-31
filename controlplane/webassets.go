// SPDX-License-Identifier: Apache-2.0
package main

import (
	"embed"
	"io/fs"
	"net/http"
)

// web/dist is the built React dashboard (controlplane/web) — committed
// to git like bpf/flow_cgroup.o in the agent module, for the same reason:
// `go build`/`go test` should work out of the box without requiring
// Node/npm to be installed. Rebuild and re-commit it after any change
// under web/src (`cd web && npm run build`).
//
//go:embed web/dist
var webDist embed.FS

// webAssetsHandler serves the embedded SPA at "/", falling back to
// index.html for any path that isn't a real asset — the client-side
// router (currently none, single page) would otherwise 404 on a
// deep-link/refresh.
func webAssetsHandler() http.Handler {
	sub, err := fs.Sub(webDist, "web/dist")
	if err != nil {
		panic("embedded web/dist is missing or malformed: " + err.Error())
	}

	fileServer := http.FileServer(http.FS(sub))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := fs.Stat(sub, r.URL.Path[1:]); err != nil {
			r = r.Clone(r.Context())
			r.URL.Path = "/"
		}
		fileServer.ServeHTTP(w, r)
	})
}
