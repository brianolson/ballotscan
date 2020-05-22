package main

import (
	"fmt"
	"hash/fnv"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	cbor "github.com/brianolson/cbor_go"
)

var BytesPerImageArchiveFile uint64 = 10000000

type ImageArchiver interface {
	ArchiveImage(imbytes []byte, r *http.Request)
}

func NewFileImageArchiver(path string) (archie ImageArchiver, err error) {
	err = os.MkdirAll(path, 0755)
	if err != nil {
		return
	}
	// TODO: read previous archive file and pre-warm dup hashes
	out := &fileImageArchiver{path: path}
	err = out.loadDupCache()
	if err != nil {
		log.Printf("%s: loadDupcache warning %s", path, err.Error())
	}
	return out, nil
}

type fileImageArchiver struct {
	path string

	recentHashesCBuf    []uint64
	recentHashesMap     map[uint64]bool
	recentHashesCBufPos int

	fname string
	fpath string
	fout  io.WriteCloser
	lock  sync.Mutex

	foutBytesWritten uint64
}

type ArchiveImageMeta struct {
	Header     http.Header `cbor:"h"`
	RemoteAddr string      `cbor:"a"`
	Timestamp  int64       `cbor:"t"` // Java-time milliseconds since 1970
}

type ArchiveImageRecord struct {
	Meta  ArchiveImageMeta `cbor:"m"`
	Image []byte           `cbor:"i"`
}

func JavaTime() int64 {
	now := time.Now()
	return (now.Unix() * 1000) + int64(now.Nanosecond()/1000000)
}

func (fia *fileImageArchiver) ArchiveImage(imbytes []byte, r *http.Request) {
	fia.lock.Lock()
	if fia.isDup(imbytes) {
		fia.lock.Unlock()
		return
	}
	fia.lock.Unlock()
	rec := ArchiveImageRecord{
		Meta: ArchiveImageMeta{
			Header:     r.Header,
			RemoteAddr: r.RemoteAddr,
			Timestamp:  JavaTime(),
		},
		Image: imbytes,
	}
	recbytes, err := cbor.Dumps(rec)
	if err != nil {
		log.Printf("ArchiveImage cbor dumps %s", err.Error())
		return
	}
	fia.lock.Lock()
	defer fia.lock.Unlock()
	if fia.fout == nil || fia.foutBytesWritten > BytesPerImageArchiveFile {
		err = fia.newFout()
		if err != nil {
			fia.fout = nil
			log.Printf("%s: ArchiveImage new %s", fia.fpath, err.Error())
			return
		}
	}
	_, err = fia.fout.Write(recbytes)
	if err != nil {
		fia.fout.Close()
		fia.fout = nil
		log.Printf("%s: ArchiveImage write %s", fia.fpath, err.Error())
	}
}

func (fia *fileImageArchiver) isDup(imbytes []byte) bool {
	hasher := fnv.New64a()
	hasher.Write(imbytes)
	imhash := hasher.Sum64()

	if fia.recentHashesCBuf == nil {
		fia.recentHashesCBuf = make([]uint64, 4000)
		fia.recentHashesMap = make(map[uint64]bool, 4000)
	} else {
		if fia.recentHashesMap[imhash] {
			// duplicate found
			return true
		}
		fia.recentHashesCBufPos++
		delete(fia.recentHashesMap, fia.recentHashesCBuf[fia.recentHashesCBufPos])
	}
	fia.recentHashesCBuf[fia.recentHashesCBufPos] = imhash
	fia.recentHashesMap[imhash] = true
	return false
}

func (fia *fileImageArchiver) newFout() (err error) {
	if fia.fout != nil {
		fia.fout.Close()
		fia.fout = nil
	}
	fia.fname = fmt.Sprintf("ima_%d_%d.cbor", JavaTime(), rand.Int31())
	fia.fpath = filepath.Join(fia.path, fia.fname)
	fia.fout, err = os.Create(fia.fpath)
	fia.foutBytesWritten = 0
	return
}

func (fia *fileImageArchiver) loadDupCache() (err error) {
	dir, err := os.Open(fia.path)
	if err != nil {
		return
	}
	newestpath := ""
	var newestTime time.Time
	names, err := dir.Readdirnames(0)
	for _, fname := range names {
		if strings.HasPrefix(fname, "ima_") && strings.HasSuffix(fname, ".cbor") {
			fpath := filepath.Join(fia.path, fname)
			fi, err := os.Lstat(fpath)
			if err != nil {
				continue
			}
			mtime := fi.ModTime()
			if mtime.After(newestTime) {
				newestTime = mtime
				newestpath = fpath
			}
		}
	}
	if newestpath != "" {
		fin, err := os.Open(newestpath)
		if err != nil {
			return err
		}
		dec := cbor.NewDecoder(fin)
		fia.recentHashesCBuf = make([]uint64, 4000)
		fia.recentHashesMap = make(map[uint64]bool, 4000)
		fia.recentHashesCBufPos = 0
		for true {
			var rec ArchiveImageRecord
			err = dec.Decode(&rec)
			if err != nil {
				continue
			}
			hasher := fnv.New64a()
			hasher.Write(rec.Image)
			imhash := hasher.Sum64()
			fia.recentHashesCBuf[fia.recentHashesCBufPos] = imhash
			fia.recentHashesMap[imhash] = true
		}
	}
	return nil
}
