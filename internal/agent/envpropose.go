package agent

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/ly-solutions/harnessly/internal/hubproto"
)

// A change of the organization's environment the runtime proposes for the machine's person (hubproto.EnvProposal,
// HRN-1329): POST /env/propose on the local port, with the local token, is sent to the machine's own hub as the event
// env.propose and answered with what the hub said (op env.proposed). The hub only keeps the proposal for an administrator
// of the organization; nothing is decided here. An observe link is not asked: that hub does not own the machine.

const (
	envProposeTO  = 45 * time.Second
	envProposeMax = 256 * 1024
)

// envProposeRoute serves /env/propose: the body is an EnvProposal without its request id.
func (a *Agent) envProposeRoute(w http.ResponseWriter, r *http.Request) {
	if !tokenMatches(r.Header.Get(a.cfg.LocalTokenHeader), a.localToken) {
		reply(w, 403, map[string]string{"error": "no local agent token (agent.json in the data directory)"})
		return
	}
	var in hubproto.EnvProposal
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, envProposeMax)).Decode(&in) != nil {
		reply(w, 400, map[string]string{"error": "bad json"})
		return
	}
	if a.hub == nil || a.hub.observe || a.hub.policy.Disabled() {
		reply(w, 200, hubproto.EnvProposalResult{Status: http.StatusConflict, Code: "no_hub", Message: "Эта машина не подключена к хабу, окружения организации у неё нет"})
		return
	}
	in.RequestID = newRequestID()
	frame, err := marshal(struct {
		Event string `json:"event"`
		hubproto.EnvProposal
	}{hubproto.EventEnvPropose, in})
	if err != nil {
		reply(w, 500, map[string]string{"error": "not encoded"})
		return
	}
	res := a.ask(in.RequestID, envProposeTO, func() bool {
		links := a.hub.hubLinks()
		if len(links) == 0 {
			return false
		}
		links[0].sendEvent(hubproto.EventEnvPropose, frame)
		return true
	})
	out := hubproto.EnvProposalResult{RequestID: in.RequestID, Status: res.Status}
	if res.Status == http.StatusOK && json.Unmarshal(res.Body, &out) == nil && out.Status != 0 {
		reply(w, 200, out)
		return
	}
	out.Status, out.Code, out.Message = http.StatusServiceUnavailable, "hub_unreachable", "Хаб не ответил, попробуйте позже"
	reply(w, 200, out)
}

// opEnvProposed takes the hub's answer to a proposal and hands it to the request that waits for it.
func (h *hub) opEnvProposed(req map[string]json.RawMessage) (any, func(), error) {
	requestID, _ := field[string](req, "requestId")
	body, err := json.Marshal(req)
	if err != nil {
		return nil, nil, invalid("the answer was not read")
	}
	h.a.settleDoc(requestID, DocResult{Status: http.StatusOK, Body: body})
	status, _ := field[float64](req, "status")
	h.audit.Record(hubproto.OpEnvProposed, true, map[string]any{"status": int(status)})
	return map[string]bool{"settled": true}, nil, nil
}
