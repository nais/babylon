package logger

import (
	"errors"

	log "github.com/sirupsen/logrus"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
)

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

func Logk8sError(err error) bool {
	var statusError *k8serrors.StatusError
	switch {
	case errors.As(err, &statusError):
		log.Errorf("Error getting deployment %v", statusError.ErrStatus.Message)

		return true
	case err != nil:
		log.Error(err.Error())

		return true
	default:
		return false
	}
}
