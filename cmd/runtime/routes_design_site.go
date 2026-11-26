package main

import (
	"context"
	"errors"

	"github.com/ly-solutions/harnessly/internal/convo"
	"github.com/ly-solutions/harnessly/internal/gate"
	"github.com/ly-solutions/harnessly/internal/peers"
	"github.com/ly-solutions/harnessly/internal/wire"
)

// A site board's comments and shared settings (slice "design"): the people's threads on the boards, the
// settings everybody sees the boards with, and the agent's side of both.

func init() {
	routeGroups = append(routeGroups, designSiteRoutes)
}

func sitePath(p string) string    { return platformAPI + "/design/site" + p }
func siteCLIPath(p string) string { return "/api/cli/design/site" + p }

func designSiteRoutes(d deps) []gate.JSONRoute {
	a := designAPI(d)
	return []gate.JSONRoute{
		designRoute(sitePath("/comments"), a.SiteComments),
		designRoute(sitePath("/comments/agent"), a.SiteCommentAgent),
		designRoute(sitePath("/props"), a.SiteProps),
		designRoute(sitePath("/pages"), a.SitePages),
		designRoute(sitePath("/shot"), a.SiteShotRoute),
		designRoute(siteCLIPath("/comments"), a.CLISiteComments),
		designRoute(siteCLIPath("/props"), a.CLISiteProps),
	}
}

// tellChat puts a comment that calls the agent into a chat of this machine, as another person's message: the agent
// reads it as theirs (peers.Envelope via person), not as the owner's words.
func tellChat(rt func() *convo.Runtime) func(ctx context.Context, conv, from, about, text string) error {
	return func(ctx context.Context, conv, from, about, text string) error {
		r := rt()
		if r == nil {
			return errors.New("the chats do not run")
		}
		id := peers.NewID()
		env := peers.Envelope{ID: id, Person: from, Chat: about, Via: "person", Text: text}
		res := r.DeliverPeer(ctx, conv, convo.PeerClientPrefix+id, peers.Wrap(env), true)
		if res.R == wire.ResultRefused {
			return errors.New("the chat did not take the comment: " + string(res.Code))
		}
		return nil
	}
}
