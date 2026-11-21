package ui

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"net/http"
)

var scriptOrder = []string{
	"gfx/theme.js",
	"model.js",
	"gfx/camera.js",
	"gfx/hit.js",
	"gfx/gl.js",
	"gfx/terrain.js",
	"gfx/bridges.js",
	"gfx/labels.js",
	"gfx/recall.js",
	"gfx/overlay.js",
	"gfx/live.js",
	"gfx/scene.js",
	"gfx/lite.js",
	"net.js",
	"nav.js",
	"view.js",
	"app.js",
}

const (
	jsType          = "text/javascript; charset=utf-8"
	loaderName      = "loader.js"
	loaderScriptTag = "__SCRIPTS__"
)

func scriptURLs() []string {
	urls := make([]string, len(scriptOrder))
	for i, name := range scriptOrder {
		urls[i] = "/" + name
	}
	return urls
}

func (s *Server) scriptRoutes() []route {
	routes := make([]route, 0, len(scriptOrder)+1)
	for _, name := range scriptOrder {
		routes = append(routes, route{"/" + name, s.asset(name, jsType)})
	}
	routes = append(routes, route{"/" + loaderName, s.loader()})
	return routes
}

func (s *Server) loader() func(http.ResponseWriter, *http.Request) {
	data, err := fs.ReadFile(assetFS, "assets/"+loaderName)
	list, jerr := json.Marshal(scriptURLs())
	if err == nil && jerr == nil {
		data = bytes.Replace(data, []byte(loaderScriptTag), list, 1)
	}
	return func(w http.ResponseWriter, _ *http.Request) {
		if err != nil || jerr != nil {
			writeError(w, http.StatusInternalServerError, "asset missing")
			return
		}
		w.Header().Set("Content-Type", jsType)
		_, _ = w.Write(data)
	}
}
