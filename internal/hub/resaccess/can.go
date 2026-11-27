package resaccess

// can is the one table of «role × action» for every kind of object: a design, a document, a board, an
// artifact, a chat, a kit, an app. It is the table of the target architecture's board (section «Поделиться»), and
// TestRoleActionMatrix is that table cell by cell:
//
//	role        view  comment  edit  share                 general  transfer
//	view         ✓      —       —     —                      —        —
//	comment      ✓      ✓       —     —                      —        —
//	edit         ✓      ✓       ✓     if the owner lets      —        —
//	manage       ✓      ✓       ✓     ✓                      ✓        —
//	owner        ✓      ✓       ✓     ✓                      ✓        ✓
//
// Whoever may share may edit: no role runs the access of an object it may not change. The steward (the lead of the home
// team, the administrator of its organization) is a manager by this rule: the folds lift them to manage, so the
// «Поделиться» window and the «Редактировать» button never disagree (the bug of the epic: «may share, edit closed»).
// A kind narrows the table only by its ceilings (a kit: kit.go; a chat snapshot: read by its space), never widens it.

// Actions a person takes on an object.
const (
	ActView     = "view"
	ActComment  = "comment"
	ActEdit     = "edit"
	ActShare    = "share"    // invite people and teams, answer requests, make guest links
	ActGeneral  = "general"  // «Общий доступ»: anyone with the link
	ActTransfer = "transfer" // delete the object, give it to another owner
)

// Actions lists every action, in the order of the table.
var Actions = []string{ActView, ActComment, ActEdit, ActShare, ActGeneral, ActTransfer}

// Can tells whether a role may take an action; editorsCanShare is the owner's «editors may share too».
func Can(role string, editorsCanShare bool, action string) bool {
	r := Rank(role)
	switch action {
	case ActView:
		return r >= Rank(roleView)
	case ActComment:
		return r >= Rank(roleComment)
	case ActEdit:
		return r >= Rank(roleEdit)
	case ActShare:
		return r >= Rank(roleManage) || (role == roleEdit && editorsCanShare)
	case ActGeneral:
		return r >= Rank(roleManage)
	case ActTransfer:
		return role == roleOwner
	}
	return false
}

// CanAll is the answer for every action, as the views give it to the page: the buttons read it and nothing else.
func CanAll(role string, editorsCanShare bool) map[string]bool {
	out := make(map[string]bool, len(Actions))
	for _, a := range Actions {
		out[a] = Can(role, editorsCanShare, a)
	}
	return out
}

// RoleManage is the role a steward is lifted to (the folds of resaccess and designaccess).
const RoleManage = roleManage
