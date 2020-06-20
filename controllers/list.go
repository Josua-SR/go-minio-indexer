package controllers

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	. "github.com/maltegrosse/go-minio-list/log"
	"github.com/maltegrosse/go-minio-list/models"
)

// Index is the persistent directory index used by the list controller
var Index *models.DirectoryEntry = nil

// List returns either a rendered index file or forwards the target download file
func List(w http.ResponseWriter, r *http.Request) {
	var root *models.DirectoryEntry
	var leaf *models.DirectoryEntry
	var requestpath []string

	// reuse existing index, if any
	root = Index
	if Index == nil {
		Log.Info("directory index not (yet) ready")
		http.Error(w, "index not ready", http.StatusServiceUnavailable)
	}

	// look up request path in directory tree
	requestpath = make([]string, 0, strings.Count(r.URL.Path, "/"))
	for _, v := range strings.Split(r.URL.Path, "/") {
		if len(v) > 1 {
			requestpath = append(requestpath, v)
		}
	}
	Log.Info("Serving path " + strings.Join(requestpath, "/"))
	leaf = root
	for level := 0; level < len(requestpath); level++ {
		var matched bool = false
		// match this path level
		for _, f := range leaf.Files {
			Log.Info("Have entry " + leaf.Name + " at " + strings.Join(leaf.Path, "/"))
			if f.Name == requestpath[level] {
				// have a match
				leaf = f
				matched = true

				// continue on next level, if any
				break
			}

			// try next entry
			continue
		}

		if !matched {
			// no match :(
			http.Error(w, "file/folder not found", http.StatusNotFound)
			return
		}
	}

	// getting here means the path was matched!
	// TODO: is trailing / a problem?

	// special handling for files
	if leaf.Type == models.DEFile {
		Log.Debug("Serving file " + leaf.Name + " at " + strings.Join(leaf.Path, "/"))

		// allow some caching as files are rather static
		w.Header().Set("Cache-Control", "max-age:600, public")

		// either proxy the download
		if models.ReverseProxy {
			uri, _ := url.Parse(models.DirectUrl + r.URL.Path)
			proxy := httputil.ReverseProxy{Director: func(r *http.Request) {
				r.URL.Scheme = uri.Scheme
				r.URL.Host = uri.Host
				r.URL.Path = uri.Path
				r.Host = uri.Host
			}}
			proxy.ServeHTTP(w, r)
		} else {
			// or redirect to S3
			http.Redirect(w, r, models.PublicUrl+r.URL.Path, http.StatusTemporaryRedirect)
		}

		// done
		return
	}

	// special handling for folders
	if leaf.Type == models.DEFolder {
		Log.Info("Serving folder " + leaf.Name + " at " + strings.Join(leaf.Path, "/"))

		// allow very cautious caching
		w.Header().Set("Cache-Control", "max-age:60, public")

		// render the html template
		res, err := leaf.RenderHTML()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// and return the rendered html file
		_, err = w.Write(res)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// all good
		return
	}

	// getting here means that a new DirectoryEntry type has been used
	Log.Error("internal error handling http request: invalid directory entry")
	http.Error(w, "internal error", http.StatusInternalServerError)
	return
}
