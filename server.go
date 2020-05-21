package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"sync"
)

type ScanServer struct {
	bubbleCache     map[int64]*BubblesJson
	pngCache        map[int64][]byte
	bubbleCacheLock sync.Mutex

	appPrefix    string
	studioPrefix string

	getter *http.Client
}

func NewScanServer() *ScanServer {
	out := new(ScanServer)
	out.bubbleCache = make(map[int64]*BubblesJson)
	out.pngCache = make(map[int64][]byte)
	out.appPrefix = ""
	out.studioPrefix = ""
	out.getter = http.DefaultClient
	return out
}

func textResponse(w http.ResponseWriter, code int, text string) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(code)
	w.Write([]byte(text))
}

func (ss *ScanServer) studioUrl(suffix string) string {
	ub, err := url.Parse(ss.studioPrefix)
	if err != nil {
		panic(err)
	}
	ub.Path = path.Join(ub.Path, suffix)
	return ub.String()
}

// Looks up bubbles.json from ballotstudio service {studioPrefix}/election/{electionid}_bubbles.json
func (ss *ScanServer) getBubbles(electionid int64) (bj *BubblesJson, err error) {
	// two small lock windows. do _not_ hold the lock during potentially slow HTTP GET
	ss.bubbleCacheLock.Lock()
	var ok bool
	bj, ok = ss.bubbleCache[electionid]
	ss.bubbleCacheLock.Unlock()
	if ok {
		return bj, nil
	}
	url := ss.studioUrl(fmt.Sprintf("/election/%d_bubbles.json", electionid))
	response, err := ss.getter.Get(url)
	if err != nil {
		return nil, fmt.Errorf("GET %v, %s", url, err.Error())
	}
	contentType := response.Header.Get("Content-Type")
	if contentType != "application/json" {
		return nil, fmt.Errorf("bubbles not json but %#v", contentType)
	}
	jd := json.NewDecoder(response.Body)
	bj = new(BubblesJson)
	err = jd.Decode(bj)
	if err == nil {
		ss.bubbleCacheLock.Lock()
		ss.bubbleCache[electionid] = bj
		ss.bubbleCacheLock.Unlock()
	}
	return bj, err
}

// Looks up pallot png from ballotstudio service {studioPrefix}/election/{electionid}.png
func (ss *ScanServer) getBallotPNG(electionid int64) (pngbytes []byte, err error) {
	// two small lock windows. do _not_ hold the lock during potentially slow HTTP GET
	ss.bubbleCacheLock.Lock()
	var ok bool
	pngbytes, ok = ss.pngCache[electionid]
	ss.bubbleCacheLock.Unlock()
	if ok {
		return pngbytes, nil
	}
	url := ss.studioUrl(fmt.Sprintf("/election/%d.png", electionid))
	response, err := ss.getter.Get(url)
	if err != nil {
		return nil, fmt.Errorf("GET %v, %s", url, err.Error())
	}
	contentType := response.Header.Get("Content-Type")
	if contentType != "image/png" {
		return nil, fmt.Errorf("not png but %#v", contentType)
	}
	pngbytes, err = ioutil.ReadAll(response.Body)
	if err == nil {
		ss.bubbleCacheLock.Lock()
		ss.pngCache[electionid] = pngbytes
		ss.bubbleCacheLock.Unlock()
	}
	return pngbytes, err
}

// {appPrefix}/scan/{electionid}
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
	if !strings.HasPrefix(path, "/scan/") {
		log.Printf("wanted path prefixed with \"/scan\" but got %v, SYSTEM MISCONFIGURED", path)
		//os.Exit(1)
		textResponse(w, http.StatusInternalServerError, "bad path")
		return
	}
	electionid, err := strconv.ParseInt(path[6:], 10, 64)
	if err != nil {
		log.Printf("bad electionid %#v", path[6:])
		textResponse(w, http.StatusBadRequest, "bad electionid")
		return
	}
	// TODO: run getBubbles in parallel with image decoding?
	// TODO: clever connection stuff to keep connection to ballotstudio service open; get bubbles, then get png
	bubbles, err := ss.getBubbles(electionid)
	if err != nil {
		log.Printf("failed to get bubbles for election %d: %s", electionid, err.Error())
		textResponse(w, http.StatusInternalServerError, "bubble lookup")
		return
	}
	pngbytes, err := ss.getBallotPNG(electionid)
	if err != nil {
		log.Printf("failed to get png for election %d: %s", electionid, err.Error())
		textResponse(w, http.StatusInternalServerError, "png lookup")
		return
	}
	orig, format, err := image.Decode(bytes.NewReader(pngbytes))
	if err != nil {
		log.Printf("bad png decode %d: %v %s", electionid, format, err.Error())
		textResponse(w, http.StatusInternalServerError, "png decode")
		return
	}

	var s Scanner
	s.bj = *bubbles
	s.setOrigImage(orig)

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
		if isImage(part.Header.Get("Content-Type")) {
			im, format, err := image.Decode(part)
			if err != nil {
				log.Printf("bad image decode cd=%v fn=%v form=%v format=%v err=%v", part.Header.Get("Content-Disposition"), part.FileName(), part.FormName(), format, err)
				textResponse(w, http.StatusBadRequest, "bad image")
				return
			}
			err = s.processScannedImage(im)
			if err != nil {
				textResponse(w, http.StatusBadRequest, err.Error())
				return
			}
			textResponse(w, http.StatusInternalServerError, "TODO WRITEME actually return structured data of which bubbles were filled")
			return
		}
	}
	textResponse(w, http.StatusBadRequest, "no image?")
}

func isImage(contentType string) bool {
	return strings.HasPrefix(contentType, "image/")
}
