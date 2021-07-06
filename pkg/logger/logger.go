package logger

import log "github.com/sirupsen/logrus"

func Setup(level string) {
	log.SetFormatter(&log.JSONFormatter{FieldMap: log.FieldMap{
		log.FieldKeyMsg: "message",
	}})

	l, err := log.ParseLevel(level)
	if err != nil {
		log.Fatal(err)
	}

	log.SetLevel(l)
}
