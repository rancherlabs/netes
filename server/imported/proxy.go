package imported

import (
	"crypto/tls"
	"io"
	"net/http"

	log "github.com/Sirupsen/logrus"
)

func (s *importedServer) proxyRequest(rw http.ResponseWriter, req *http.Request) {
	if len(req.Header.Get("Upgrade")) > 0 {
		s.proxyRequestWebsocket(rw, req)
	} else {
		s.proxy.ServeHTTP(rw, req)
	}
}

func (s *importedServer) proxyRequestWebsocket(rw http.ResponseWriter, req *http.Request) {
	// Inspired by https://groups.google.com/forum/#!searchin/golang-nuts/httputil.ReverseProxy$20$2B$20websockets/golang-nuts/KBx9pDlvFOc/01vn1qUyVdwJ
	d, err := tls.Dial("tcp", s.address, s.tlsConfig)
	if err != nil {
		log.WithField("error", err).Error("Error dialing websocket backend.")
		http.Error(rw, "Unable to establish websocket connection: can't dial.", 500)
		return
	}
	hj, ok := rw.(http.Hijacker)
	if !ok {
		http.Error(rw, "Unable to establish websocket connection: no hijacker.", 500)
		return
	}
	nc, _, err := hj.Hijack()
	if err != nil {
		log.WithField("error", err).Error("Hijack error.")
		http.Error(rw, "Unable to establish websocket connection: can't hijack.", 500)
		return
	}
	defer nc.Close()
	defer d.Close()

	err = req.Write(d)
	if err != nil {
		log.WithField("error", err).Error("Error copying request to target.")
		return
	}

	errc := make(chan error, 2)
	cp := func(dst io.Writer, src io.Reader) {
		_, err := io.Copy(dst, src)
		errc <- err
	}
	go cp(d, nc)
	go cp(nc, d)
	<-errc
}
