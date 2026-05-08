package ui

import (
	"encoding/base64"

	"fyne.io/fyne/v2"
)

const portshareIconPNGBase64 = "iVBORw0KGgoAAAANSUhEUgAAABAAAAAQCAYAAAAf8/9hAAAAAXNSR0IArs4c6QAAAARnQU1BAACxjwv8YQUAAAAJcEhZcwAADsMAAA7DAcdvqGQAAAAsSURBVDhPYxCRVP5PCWZAFyAVU9cAl77/ROGRYAAuQD8DCOHhbAA5eOANAAB1WfOEdhKUUgAAAABJRU5ErkJggg=="

func portshareIconResource() fyne.Resource {
	data, err := base64.StdEncoding.DecodeString(portshareIconPNGBase64)
	if err != nil {
		panic(err)
	}
	return fyne.NewStaticResource("portshare.png", data)
}
