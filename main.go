package main

// vouch
// github.com/vouch/vouch-proxy

import (
	"log"
	"net/http"
	"path/filepath"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"go.uber.org/zap"

	"github.com/vouch/vouch-proxy/handlers"
	"github.com/vouch/vouch-proxy/pkg/cfg"
	"github.com/vouch/vouch-proxy/pkg/timelog"
	tran "github.com/vouch/vouch-proxy/pkg/transciever"
)

// version and semver get overwritten by build with
// go build -i -v -ldflags="-X main.version=$(git describe --always --long) -X main.semver=v$(git semver get)"
var (
	version   = "undefined"
	builddt   = "undefined"
	host      = "undefined"
	semver    = "undefined"
	branch    = "undefined"
	staticDir = "/static/"
	logger    = cfg.Cfg.Logger
	fastlog   = cfg.Cfg.FastLogger
)

// fwdToZapWriter allows us to use the zap.Logger as our http.Server ErrorLog
// see https://stackoverflow.com/questions/52294334/net-http-set-custom-logger
type fwdToZapWriter struct {
	logger *zap.Logger
}

func (fw *fwdToZapWriter) Write(p []byte) (n int, err error) {
	fw.logger.Error(string(p))
	return len(p), nil
}

func main() {
	var listen = cfg.Cfg.Listen + ":" + strconv.Itoa(cfg.Cfg.Port)
	logger.Infow("starting "+cfg.Branding.CcName,
		// "semver":    semver,
		"version", version,
		"buildtime", builddt,
		"buildhost", host,
		"branch", branch,
		"semver", semver,
		"listen", listen,
		"oauth.provider", cfg.GenOAuth.Provider)

	mux := mux.NewRouter()

	authH := http.HandlerFunc(handlers.ValidateRequestHandler)
	mux.HandleFunc("/validate", timelog.TimeLog(authH))
	mux.HandleFunc("/_external-auth-{id}", timelog.TimeLog(authH))

	loginH := http.HandlerFunc(handlers.LoginHandler)
	mux.HandleFunc("/login", timelog.TimeLog(loginH))

	logoutH := http.HandlerFunc(handlers.LogoutHandler)
	mux.HandleFunc("/logout", timelog.TimeLog(logoutH))

	callH := http.HandlerFunc(handlers.CallbackHandler)
	mux.HandleFunc("/auth", timelog.TimeLog(callH))

	healthH := http.HandlerFunc(handlers.HealthcheckHandler)
	mux.HandleFunc("/healthcheck", timelog.TimeLog(healthH))

	if logger.Desugar().Core().Enabled(zap.DebugLevel) {
		path, err := filepath.Abs(staticDir)
		if err != nil {
			logger.Errorf("couldn't find static assets at %s", path)
		}
		logger.Debugf("serving static files from %s", path)
	}
	// https://golangcode.com/serve-static-assets-using-the-mux-router/
	mux.PathPrefix(staticDir).Handler(http.StripPrefix(staticDir, (http.FileServer(http.Dir("." + staticDir)))))

	if cfg.Cfg.WebApp {
		logger.Info("enabling websocket")
		tran.ExplicitInit()
		mux.Handle("/ws", tran.WS)
	}

	// socketio := tran.NewServer()
	// mux.Handle("/socket.io/", cors.AllowAll(socketio))
	// http.Handle("/socket.io/", tran.Server)

	srv := &http.Server{
		Handler: mux,
		Addr:    listen,
		// Good practice: enforce timeouts for servers you create!
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
		ErrorLog:     log.New(&fwdToZapWriter{fastlog}, "", 0),
	}

	log.Fatal(srv.ListenAndServe())

}
