package automations

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/ly-solutions/harnessly/internal/hub/httpx"
	"github.com/ly-solutions/harnessly/internal/hub/store"
	"github.com/ly-solutions/harnessly/internal/hubproto"
)

// MachineCall is a routines call from the owner's machine (POST /agent/routines): the `harnessly routine` command and
// the routine tools of its agents. owner is the machine's owner; the routine is named by id or by name.
func (s *Service) MachineCall(ctx context.Context, owner, email, machine string, call hubproto.RoutinesCall) (any, error) {
	if !s.Enabled() {
		return nil, errOff
	}
	find := func() (Rule, error) {
		id := strings.TrimSpace(call.ID)
		if id == "" {
			return Rule{}, invalid("Назови задачу: id или имя")
		}
		r, err := s.ruleOf(ctx, id, owner)
		if err == store.ErrNotFound {
			r, err = scanRule(s.db.QueryRow(ctx, `SELECT `+ruleCols+` FROM automation_rules WHERE owner_id = ? AND name = ?`, owner, id))
		}
		if err == nil && r.Routine == nil {
			err = store.ErrNotFound
		}
		return ruleOrNotFound(r, err)
	}
	body := func() (ruleBody, error) {
		var b ruleBody
		if len(call.Body) == 0 {
			return b, invalid("Нужно тело задачи")
		}
		dec := json.NewDecoder(strings.NewReader(string(call.Body)))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&b); err != nil {
			return b, invalid("Тело задачи: " + err.Error())
		}
		return b, nil
	}
	switch call.Op {
	case "list":
		views, err := s.routineViews(ctx, owner)
		if err != nil {
			return nil, err
		}
		return map[string]any{"routines": views, "defaultModel": s.routineModel(Rule{})}, nil
	case "get":
		r, err := find()
		if err != nil {
			return nil, err
		}
		views, err := s.routineViews(ctx, owner)
		if err != nil {
			return nil, err
		}
		for _, v := range views {
			if v.ID == r.ID {
				return map[string]any{"routine": v}, nil
			}
		}
		return nil, errNoRule
	case "create":
		b, err := body()
		if err != nil {
			return nil, err
		}
		if b.Routine == nil {
			b.Routine = &Routine{}
		}
		on := true
		if b.Enabled == nil {
			b.Enabled = &on
		}
		rule := Rule{OwnerID: owner}
		if err := s.apply(ctx, &rule, b); err != nil {
			return nil, err
		}
		if rule.Routine != nil && rule.Routine.Machine == "" {
			rule.Routine.Machine = machine
		}
		rule, ok, err := s.addRule(ctx, rule)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, errTaken
		}
		return map[string]any{"routine": rule, "next": nextSlots(rule, s.now(), 3), "url": strings.TrimRight(s.cfg.PublicURL, "/") + "/routines/" + rule.ID}, nil
	case "update":
		rule, err := find()
		if err != nil {
			return nil, err
		}
		b, err := body()
		if err != nil {
			return nil, err
		}
		if err := s.apply(ctx, &rule, b); err != nil {
			return nil, err
		}
		if err := s.saveRule(ctx, rule); err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "unique") || strings.Contains(strings.ToLower(err.Error()), "duplicate") {
				return nil, errTaken
			}
			return nil, err
		}
		rule, err = s.ruleOf(ctx, rule.ID, owner)
		if err != nil {
			return nil, err
		}
		return map[string]any{"routine": rule, "next": nextSlots(rule, s.now(), 3)}, nil
	case "run":
		rule, err := find()
		if err != nil {
			return nil, err
		}
		// A run by hand is wanted where the person is: the routine follows them to this machine.
		if rule.Routine.Machine != machine && machine != "" {
			rule.Routine.Machine = machine
			if err := s.saveRule(ctx, rule); err != nil {
				return nil, err
			}
		}
		return s.queueManual(ctx, rule, email)
	case "delete":
		rule, err := find()
		if err != nil {
			return nil, err
		}
		gone, err := s.deleteRule(ctx, rule.ID, owner)
		if err != nil {
			return nil, err
		}
		return map[string]any{"deleted": gone}, nil
	case "runs":
		rule, err := find()
		if err != nil {
			return nil, err
		}
		limit := call.Limit
		if limit <= 0 || limit > 100 {
			limit = 20
		}
		runs, err := s.runs(ctx, owner, rule.ID, limit)
		if err != nil {
			return nil, err
		}
		return map[string]any{"runs": runs}, nil
	case "chat":
		return s.routineOfChat(ctx, owner, strings.TrimSpace(call.ID), call.Limit)
	}
	return nil, httpx.New(http.StatusBadRequest, "invalid_request", "op: list, get, create, update, run, delete, runs или chat")
}

// routineOfChat is the routine whose run a conversation is (op chat): the routine as the list shows it, the
// run, and its last runs newest first, for the routines app in that chat. A chat that is no run of the owner's
// routine answers {routine: null}: the app then shows nothing, and the page cannot tell a stranger's run from none.
func (s *Service) routineOfChat(ctx context.Context, owner, conversation string, limit int) (any, error) {
	none := map[string]any{"routine": nil}
	if conversation == "" || len(conversation) > 200 {
		return none, nil
	}
	run, err := scanRun(s.db.QueryRow(ctx, `SELECT `+prefixCols("r.", runCols)+` FROM automation_runs r JOIN automation_rules ar ON ar.id = r.rule_id
		WHERE r.conversation_id = ? AND ar.owner_id = ? ORDER BY r.created_at DESC LIMIT 1`, conversation, owner))
	if err == store.ErrNotFound {
		return none, nil
	}
	if err != nil {
		return nil, err
	}
	views, err := s.routineViews(ctx, owner)
	if err != nil {
		return nil, err
	}
	for _, v := range views {
		if v.ID != run.RuleID {
			continue
		}
		if limit <= 0 || limit > 100 {
			limit = 20
		}
		runs, err := s.runs(ctx, owner, v.ID, limit)
		if err != nil {
			return nil, err
		}
		return map[string]any{"routine": v, "run": run, "runs": runs}, nil
	}
	// A trigger's run, not a routine's.
	return none, nil
}
