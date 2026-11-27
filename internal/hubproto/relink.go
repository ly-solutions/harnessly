package hubproto

// CodeRelinkRequired is the code of a refusal of a machine's request in its owner's name: the owner did not accept the
// machine's keys (the machine was given to them, or registered with somebody else's code). The owner accepting the
// machine in the hub, or registering it again with a code of their own, lifts it.
const CodeRelinkRequired = "machine_relink_required"
