package utils

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var MyInt int

func init() {
	wd, err := os.Getwd()
	if err != nil {
		wd = ""
		return
	}

	consoleWriter := zerolog.ConsoleWriter{
		Out:        os.Stdout,
		TimeFormat: "15:04:05", // Custom time format (e.g., HH:mm:ss)
		// FormatLevel: func(i interface{}) string {
		// 	return strings.ToUpper(fmt.Sprintf("[%s]", i))
		// },
		FormatCaller: func(i interface{}) string {
			path := i.(string)
			relPath, err := filepath.Rel(wd, path)
			if err != nil {
				relPath = path
			}
			return fmt.Sprintf("[%s]", relPath)
		},
		NoColor: false, // Set to true to disable colors
	}
	log.Logger = zerolog.New(consoleWriter).
		Level(zerolog.DebugLevel).
		With().
		Timestamp().
		Caller().
		Logger()
	log.Info().Msg("init logger")

}
