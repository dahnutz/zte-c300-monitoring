package handler

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	apperrors "zte-c300-monitoring/internal/errors"
	"zte-c300-monitoring/internal/reqctx"
	"zte-c300-monitoring/internal/store"
	"zte-c300-monitoring/internal/utils"
)

// HistoryHandler serves durable ONU samples and collection-run cost data.
type HistoryHandler struct {
	Store store.Store
}

func NewHistoryHandler(st store.Store) *HistoryHandler {
	return &HistoryHandler{Store: st}
}

func (h *HistoryHandler) ListSamples(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.Store == nil {
		utils.HandleError(w, r, apperrors.NewServiceUnavailableError("history store is not configured", nil))
		return
	}
	deviceID := reqctx.DeviceIDFromContext(r.Context())
	if deviceID == "" {
		utils.HandleError(w, r, apperrors.NewValidationError("device id missing", nil))
		return
	}
	q, err := parseSampleQuery(r, deviceID)
	if err != nil {
		utils.HandleError(w, r, err)
		return
	}
	if q.CountBy != "" {
		result, err := h.Store.ListSampleCounts(r.Context(), q)
		if err != nil {
			handleHistoryStoreError(w, r, q, err)
			return
		}
		utils.SendJSONResponse(w, http.StatusOK, utils.WebResponse{
			Code:   http.StatusOK,
			Status: "success",
			Data:   result,
		})
		return
	}
	rows, err := h.Store.ListSamples(r.Context(), q)
	if err != nil {
		handleHistoryStoreError(w, r, q, err)
		return
	}
	if rows == nil {
		rows = []store.ONUSample{}
	}
	states, err := h.Store.ListONUStates(r.Context(), deviceID)
	if err != nil {
		utils.HandleError(w, r, err)
		return
	}
	store.AttachONUState(rows, states)
	utils.SendJSONResponse(w, http.StatusOK, utils.WebResponse{
		Code:   http.StatusOK,
		Status: "success",
		Data:   rows,
	})
}

func (h *HistoryHandler) ListStatusEvents(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w, r) {
		return
	}
	deviceID := reqctx.DeviceIDFromContext(r.Context())
	q := parseEventQuery(r, deviceID)
	rows, err := h.Store.ListStatusEvents(r.Context(), q)
	if err != nil {
		utils.HandleError(w, r, err)
		return
	}
	if rows == nil {
		rows = []store.StatusEvent{}
	}
	utils.SendJSONResponse(w, http.StatusOK, utils.WebResponse{Code: http.StatusOK, Status: "success", Data: rows})
}

func (h *HistoryHandler) ListEthEvents(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w, r) {
		return
	}
	deviceID := reqctx.DeviceIDFromContext(r.Context())
	q := parseEventQuery(r, deviceID)
	rows, err := h.Store.ListEthEvents(r.Context(), q)
	if err != nil {
		utils.HandleError(w, r, err)
		return
	}
	if rows == nil {
		rows = []store.EthEvent{}
	}
	utils.SendJSONResponse(w, http.StatusOK, utils.WebResponse{Code: http.StatusOK, Status: "success", Data: rows})
}

func (h *HistoryHandler) ListUnauth(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w, r) {
		return
	}
	deviceID := reqctx.DeviceIDFromContext(r.Context())
	list, err := h.Store.ListUnauth(r.Context(), deviceID)
	if err != nil {
		utils.HandleError(w, r, err)
		return
	}
	if list.ONUs == nil {
		list.ONUs = []store.UnauthONU{}
	}
	utils.SendJSONResponse(w, http.StatusOK, utils.WebResponse{Code: http.StatusOK, Status: "success", Data: list})
}

func (h *HistoryHandler) ready(w http.ResponseWriter, r *http.Request) bool {
	if h == nil || h.Store == nil {
		utils.HandleError(w, r, apperrors.NewServiceUnavailableError("history store is not configured", nil))
		return false
	}
	if reqctx.DeviceIDFromContext(r.Context()) == "" {
		utils.HandleError(w, r, apperrors.NewValidationError("device id missing", nil))
		return false
	}
	return true
}

func parseEventQuery(r *http.Request, deviceID string) store.EventQuery {
	q := store.EventQuery{
		DeviceID: deviceID,
		Serial:   strings.TrimSpace(r.URL.Query().Get("serial")),
		Limit:    atoiDefault(r.URL.Query().Get("limit"), 50),
	}
	if raw := r.URL.Query().Get("port"); raw != "" {
		q.Port = atoiDefault(raw, 0)
	}
	return q
}

func (h *HistoryHandler) ListCollectionRuns(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.Store == nil {
		utils.HandleError(w, r, apperrors.NewServiceUnavailableError("history store is not configured", nil))
		return
	}
	deviceID := reqctx.DeviceIDFromContext(r.Context())
	if deviceID == "" {
		utils.HandleError(w, r, apperrors.NewValidationError("device id missing", nil))
		return
	}
	limit := atoiDefault(r.URL.Query().Get("limit"), 50)
	rows, err := h.Store.ListCollectionRuns(r.Context(), deviceID, limit)
	if err != nil {
		utils.HandleError(w, r, err)
		return
	}
	if rows == nil {
		rows = []store.CollectionRun{}
	}
	utils.SendJSONResponse(w, http.StatusOK, utils.WebResponse{
		Code:   http.StatusOK,
		Status: "success",
		Data:   rows,
	})
}

func parseSampleQuery(r *http.Request, deviceID string) (store.SampleQuery, error) {
	q := store.SampleQuery{
		DeviceID: deviceID,
		Serial:   strings.TrimSpace(r.URL.Query().Get("serial")),
		Status:   strings.TrimSpace(r.URL.Query().Get("status")),
		CountBy:  strings.TrimSpace(r.URL.Query().Get("count_by")),
	}
	if !store.ValidCountBy(q.CountBy) {
		return q, apperrors.NewValidationError(
			"count_by must be status, board, pon, onu_type or eth_link",
			map[string]any{"count_by": q.CountBy},
		)
	}
	if raw := r.URL.Query().Get("board"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			return q, apperrors.NewValidationError("board must be a positive integer", map[string]any{"board": raw})
		}
		q.Board = n
	}
	if raw := r.URL.Query().Get("pon"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			return q, apperrors.NewValidationError("pon must be a positive integer", map[string]any{"pon": raw})
		}
		q.PON = n
	}
	if raw := r.URL.Query().Get("onu_id"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			return q, apperrors.NewValidationError("onu_id must be a positive integer", map[string]any{"onu_id": raw})
		}
		q.ONUID = n
	}
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			return q, apperrors.NewValidationError("limit must be a positive integer", map[string]any{"limit": raw})
		}
		q.Limit = n
	}
	if raw := r.URL.Query().Get("from"); raw != "" {
		from, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return q, apperrors.NewValidationError("from must be RFC3339", map[string]any{"from": raw})
		}
		q.From = from
	}
	if raw := r.URL.Query().Get("to"); raw != "" {
		to, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return q, apperrors.NewValidationError("to must be RFC3339", map[string]any{"to": raw})
		}
		q.To = to
	}
	switch raw := strings.TrimSpace(r.URL.Query().Get("run")); raw {
	case "":
	case "latest", "last":
		q.LatestRun = true
	default:
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || id < 1 {
			return q, apperrors.NewValidationError("run must be latest, last, or a positive id", map[string]any{"run": raw})
		}
		q.RunID = id
	}
	return q, nil
}

func handleHistoryStoreError(w http.ResponseWriter, r *http.Request, q store.SampleQuery, err error) {
	if errors.Is(err, store.ErrNotFound) {
		id := any("latest")
		if q.RunID != 0 {
			id = q.RunID
		}
		utils.HandleError(w, r, apperrors.NewNotFoundError("collection run", id))
		return
	}
	utils.HandleError(w, r, err)
}

func atoiDefault(raw string, fallback int) int {
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return n
}
