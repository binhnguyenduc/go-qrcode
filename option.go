package qrcode

import (
	"image/color"
)

type Option func(q *QRCode)

func Width(w int) Option {
	return func(q *QRCode) {
		q.width = w
	}
}

func Height(h int) Option {
	return func(q *QRCode) {
		q.height = h
	}
}

func Margin(m int) Option {
	return func(q *QRCode) {
		q.margin = m
	}
}

func QuitZoneSize(s int) Option {
	return func(q *QRCode) {
		q.version.setQuietZoneSize(s)
	}
}

func ForegroundColor(c color.Color) Option {
	return func(q *QRCode) {
		if nil == c {
			q.ForegroundColor = color.Black
			return
		}
		q.ForegroundColor = c
	}
}

func BackgroundColor(c color.Color) Option {
	return func(q *QRCode) {
		if nil == c {
			q.BackgroundColor = color.White
			return
		}
		q.BackgroundColor = c
	}
}

func Level(l RecoveryLevel) Option {
	return func(q *QRCode) {
		q.level = l
	}
}

func Version(v int) Option {
	return func(q *QRCode) {
		q.VersionNumber = v
	}
}
