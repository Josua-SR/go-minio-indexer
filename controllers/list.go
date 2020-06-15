package controllers

import (
	"github.com/maltegrosse/go-minio-list/models"
	"net/http"
	"net/http/httputil"
	"net/url"
)

// List returns either a rendered index file or forwards the target download file
func List(w http.ResponseWriter, r *http.Request) {
	var dl models.DirList
	dl.Path = r.URL.Path
	// scan the current path
	err := dl.ScanPath(r.URL.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	// repair url if someone misses a /
	if dl.Redirect {
		http.Redirect(w, r, r.URL.Path+"/", http.StatusMovedPermanently)
	}
	// if the path ends in a file
	if dl.IsFile {
		// either use reverse proxy
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
			// or just redirect
			http.Redirect(w, r, models.PublicUrl+r.URL.Path, http.StatusTemporaryRedirect)
		}

	} else {
		// if no files/folders scanned and not in root path in an empty bucket
		if len(dl.Folders) < 1 && len(dl.Files) < 1 && r.URL.Path != "/" {
			http.Error(w, "file/folder not found", http.StatusNotFound)
			return
		}
		// if all fine, render the html template
		res, err := dl.RenderHtml()
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
	}

}
