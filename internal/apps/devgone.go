package apps

import (
	"errors"
	"io/fs"
	"os"
)

// DevSourceGone says a dev install points at a folder that is not there any more: a scratch folder a reboot
// wiped, a checkout that was removed. The install is still listed and can still be turned on in a chat, but nothing of
// it can be served: its views never load and its window never reports, which looked like a window "not connected".
func (a Installed) DevSourceGone() bool {
	if !a.Dev || a.Source == "" {
		return false
	}
	_, err := os.Stat(a.Source)
	return errors.Is(err, fs.ErrNotExist)
}

// devSourceGoneWords is what the host says instead of serving such an install, and what to do.
func devSourceGoneWords(a Installed) string {
	return "the folder of " + a.ID + " is gone: it is a dev install of " + a.Source + ", which is not there any more, so its " +
		"views and tools cannot load. Run `harnessly app dev <folder>` again from where the app's source is now"
}
