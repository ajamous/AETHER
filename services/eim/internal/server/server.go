// Package server is the eIM HTTP transport.
//
// Operator endpoints under /v1/devices/...; IPA-side endpoints under
// /v1/ipa/{eid}/... so an ingress can route them to different
// auth realms (operator OIDC vs device mTLS).
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	eimv1 "github.com/ajamous/aether/services/eim/api/v1"
	"github.com/ajamous/aether/services/eim/internal/devices"
)

type Server struct {
	store devices.Store
}

func New(s devices.Store) *Server { return &Server{store: s} }

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	// Operator
	mux.HandleFunc("POST /v1/devices", s.handleRegisterDevice)
	mux.HandleFunc("GET /v1/devices", s.handleListDevices)
	mux.HandleFunc("GET /v1/devices/{eid}", s.handleGetDevice)
	mux.HandleFunc("DELETE /v1/devices/{eid}", s.handleDeleteDevice)
	mux.HandleFunc("POST /v1/devices/{eid}/commands", s.handleEnqueueCommand)
	mux.HandleFunc("GET /v1/devices/{eid}/commands", s.handleListCommandsAdmin)

	// IPA-side
	mux.HandleFunc("GET /v1/ipa/{eid}/poll", s.handleIPAPoll)
	mux.HandleFunc("POST /v1/ipa/{eid}/commands/{command_id}/ack", s.handleIPAAck)

	mux.HandleFunc("GET /v1/health", s.handleHealth)
	return mux
}

func (s *Server) ListenAndServe(ctx context.Context, addr string) error {
	srv := &http.Server{Addr: addr, Handler: s.Routes(), ReadHeaderTimeout: 10 * time.Second}
	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

// --- operator handlers ----------------------------------------------------

func (s *Server) handleRegisterDevice(w http.ResponseWriter, r *http.Request) {
	var req eimv1.RegisterDeviceRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.EID == "" {
		writeProblem(w, http.StatusBadRequest, "eid required")
		return
	}
	d := &eimv1.Device{
		EID:      req.EID,
		Label:    req.Label,
		Tags:     req.Tags,
		Metadata: req.Metadata,
	}
	if err := s.store.RegisterDevice(d); err != nil {
		s.writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, d)
}

func (s *Server) handleListDevices(w http.ResponseWriter, _ *http.Request) {
	list := s.store.ListDevices()
	out := make([]eimv1.Device, 0, len(list))
	for _, d := range list {
		out = append(out, *d)
	}
	writeJSON(w, http.StatusOK, eimv1.ListDevicesResponse{Length: len(out), Devices: out})
}

func (s *Server) handleGetDevice(w http.ResponseWriter, r *http.Request) {
	d, err := s.store.GetDevice(eimv1.EID(r.PathValue("eid")))
	if err != nil {
		s.writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) handleDeleteDevice(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteDevice(eimv1.EID(r.PathValue("eid"))); err != nil {
		s.writeStoreErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleEnqueueCommand(w http.ResponseWriter, r *http.Request) {
	var req eimv1.EnqueueCommandRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Kind == "" {
		writeProblem(w, http.StatusBadRequest, "kind required")
		return
	}
	if !validCommandKind(req.Kind) {
		writeProblem(w, http.StatusBadRequest, fmt.Sprintf("unknown kind %q", req.Kind))
		return
	}
	c := &eimv1.Command{
		EID:         eimv1.EID(r.PathValue("eid")),
		Kind:        req.Kind,
		SMDPAddress: req.SMDPAddress,
		MatchingID:  req.MatchingID,
		ICCID:       req.ICCID,
	}
	if err := s.store.EnqueueCommand(c); err != nil {
		s.writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

func (s *Server) handleListCommandsAdmin(w http.ResponseWriter, r *http.Request) {
	cmds := s.store.ListCommandsForDevice(eimv1.EID(r.PathValue("eid")), true)
	out := make([]eimv1.Command, 0, len(cmds))
	for _, c := range cmds {
		out = append(out, *c)
	}
	writeJSON(w, http.StatusOK, eimv1.ListCommandsResponse{Length: len(out), Commands: out})
}

// --- IPA handlers ---------------------------------------------------------

// handleIPAPoll returns pending commands and atomically marks each
// returned command as Delivered.
func (s *Server) handleIPAPoll(w http.ResponseWriter, r *http.Request) {
	eid := eimv1.EID(r.PathValue("eid"))
	if _, err := s.store.GetDevice(eid); err != nil {
		s.writeStoreErr(w, err)
		return
	}
	cmds := s.store.ListCommandsForDevice(eid, false)
	out := make([]eimv1.Command, 0, len(cmds))
	for _, c := range cmds {
		if c.State == eimv1.CommandStatePending {
			if err := s.store.MarkDelivered(c.ID); err != nil {
				continue
			}
			c, _ = s.store.GetCommand(c.ID)
		}
		out = append(out, *c)
	}
	writeJSON(w, http.StatusOK, eimv1.ListCommandsResponse{Length: len(out), Commands: out})
}

func (s *Server) handleIPAAck(w http.ResponseWriter, r *http.Request) {
	var req eimv1.AckCommandRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := s.store.AckCommand(r.PathValue("command_id"), &req); err != nil {
		s.writeStoreErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- admin --------------------------------------------------------------

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ready": true})
}

// --- helpers ------------------------------------------------------------

func validCommandKind(k eimv1.CommandKind) bool {
	switch k {
	case eimv1.CommandDownloadProfile, eimv1.CommandEnableProfile,
		eimv1.CommandDisableProfile, eimv1.CommandDeleteProfile:
		return true
	}
	return false
}

func (s *Server) writeStoreErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, devices.ErrDeviceNotFound), errors.Is(err, devices.ErrCommandNotFound):
		writeProblem(w, http.StatusNotFound, err.Error())
	case errors.Is(err, devices.ErrDeviceExists):
		writeProblem(w, http.StatusConflict, err.Error())
	case errors.Is(err, devices.ErrInvalidArgument):
		writeProblem(w, http.StatusBadRequest, err.Error())
	default:
		writeProblem(w, http.StatusInternalServerError, err.Error())
	}
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeProblem(w, http.StatusBadRequest, fmt.Sprintf("invalid JSON: %v", err))
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeProblem(w http.ResponseWriter, status int, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"type":   "about:blank",
		"title":  http.StatusText(status),
		"status": status,
		"detail": detail,
	})
}
