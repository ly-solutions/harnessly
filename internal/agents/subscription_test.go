package agents

import "testing"

func TestASubscriptionsTokenIsToldFromAKeyOfTheAPI(t *testing.T) {
	for _, v := range []string{"sk-ant-" + "oat01-x", "  sk-ant-" + "oat01-x\n", "sk-ant-" + "ort01-x"} {
		if !ClaudeSubscriptionToken(v) {
			t.Errorf("%q is a subscription's token", v)
		}
	}
	for _, v := range []string{"", "sk-ant-" + "api03-x", "sk-" + "proj-x", "a key with sk-ant-" + "oat inside"} {
		if ClaudeSubscriptionToken(v) {
			t.Errorf("%q is no subscription's token", v)
		}
	}
}
