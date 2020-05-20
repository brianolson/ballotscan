package main

import (
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
)

type ScanServer struct {
	bubbleCache     map[string]BubblesJson
	bubbleCacheLock sync.Mutex

	appPrefix    string
	studioPrefix string
}

func NewScanServer() *ScanServer {
	out := new(ScanServer)
	out.bubbleCache = make(map[string]BubblesJson)
	out.appPrefix = ""
	out.studioPrefix = ""
	return out
}

func textResponse(w http.ResponseWriter, code int, text string) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(code)
	w.Write([]byte(text))
}

// {appPrefix}/scan/{electionid}
// Looks up bubbles.json from ballotstudio service {studioPrefix}/election/{electionid}_bubbles.json
func (ss *ScanServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if len(ss.appPrefix) > 0 {
		if strings.HasPrefix(path, ss.appPrefix) {
			path = path[len(ss.appPrefix):]
		} else {
			log.Printf("wanted path prefixed with %v but got %v, SYSTEM MISCONFIGURED", ss.appPrefix, path)
			//os.Exit(1)
			textResponse(w, http.StatusInternalServerError, "bad path")
			return
		}
	}
	if !strings.HasPrefix(path, "/scan") {
		log.Printf("wanted path prefixed with \"/scan\" but got %v, SYSTEM MISCONFIGURED", path)
		//os.Exit(1)
		textResponse(w, http.StatusInternalServerError, "bad path")
		return
	}
	mpreader, err := r.MultipartReader()
	if err != nil {
		textResponse(w, http.StatusBadRequest, err.Error())
		return
	}
	for true {
		part, err := mpreader.NextPart()
		if err == io.EOF {
			break
		} else if err != nil {
			textResponse(w, http.StatusBadRequest, err.Error())
			return
		}

		log.Printf("got part cd=%v fn=%v form=%v", part.Header.Get("Content-Disposition"), part.FileName(), part.FormName())
	}
	textResponse(w, http.StatusInternalServerError, "TODO: WRITEME")
}
