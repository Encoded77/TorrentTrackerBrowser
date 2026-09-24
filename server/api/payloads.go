package api

import (
	"io"
	"net/http"
	"strings"

	"github.com/Encoded77/TorrentTrackerBrowser/server/core"
)

type payloadView struct {
	ID       string           `json:"id"`
	Name     string           `json:"name"`
	InfoHash string           `json:"infoHash"`
	Size     *int64           `json:"size"`
	Files    []core.FileEntry `json:"files"`
}

// createPayload accepts {"magnet": ...} or a multipart "torrent" file.
func (s *Server) createPayload(w http.ResponseWriter, r *http.Request) {
	var p core.Payload
	ct := strings.ToLower(r.Header.Get("Content-Type"))
	if strings.HasPrefix(ct, "multipart/form-data") {
		r.Body = http.MaxBytesReader(w, r.Body, 16<<20)
		f, _, err := r.FormFile("torrent")
		if err != nil {
			writeError(w, http.StatusBadRequest, "missing_file", "multipart field torrent is required")
			return
		}
		defer f.Close()
		b, err := io.ReadAll(f)
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad_upload", err.Error())
			return
		}
		p, err = core.PayloadFromTorrent(b)
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad_torrent", err.Error())
			return
		}
	} else {
		var body struct {
			Magnet string `json:"magnet"`
		}
		if !readJSON(w, r, &body) {
			return
		}
		var err error
		p, err = core.PayloadFromMagnet(body.Magnet)
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad_magnet", err.Error())
			return
		}
	}
	id := s.Payloads.Add(p)
	v := payloadView{ID: id, Name: p.Name, InfoHash: p.InfoHash}
	if len(p.Torrent) > 0 {
		size := p.Size
		v.Size = &size
		v.Files = p.Files
		if v.Files == nil {
			v.Files = []core.FileEntry{}
		}
	}
	writeJSON(w, http.StatusCreated, v)
}
