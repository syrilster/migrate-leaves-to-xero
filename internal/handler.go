package internal

import (
	"bytes"
	"io"
	"net/http"
	"path/filepath"

	log "github.com/sirupsen/logrus"
	"github.com/tealeg/xlsx"

	"github.com/syrilster/migrate-leave-krow-to-xero/internal/config"
	"github.com/syrilster/migrate-leave-krow-to-xero/internal/util"
)

const supportedFileFormat = ".xlsx"

//Handler func
func Handler(xeroHandler XeroAPIHandler) func(res http.ResponseWriter, req *http.Request) {
	return func(res http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		contextLogger := log.WithContext(ctx)

		var errResult []string
		_, fileHeader, err := req.FormFile("file")
		if err != nil {
			contextLogger.WithError(err).Error("Failed to get the file from request")
			util.WithBodyAndStatus(nil, http.StatusBadRequest, res)
			return
		}

		if filepath.Ext(fileHeader.Filename) != supportedFileFormat {
			contextLogger.WithError(err).Error("Unable to open the uploaded file. Please confirm the file is in .xlsx format.")
			util.WithBodyAndStatus(nil, http.StatusBadRequest, res)
		}

		err = parseRequestBody(req)
		if err != nil {
			util.WithBodyAndStatus(nil, http.StatusInternalServerError, res)
			return
		}

		errResult = xeroHandler.MigrateLeaveKrowToXero(ctx)
		if len(errResult) > 0 {
			contextLogger.Error("There were some errors during processing leaves")
			util.WithBodyAndStatus(errResult, http.StatusInternalServerError, res)
			return
		}
		util.WithBodyAndStatus("", http.StatusOK, res)
	}
}

func parseRequestBody(req *http.Request) error {
	ctx := req.Context()
	envValues := config.NewEnvironmentConfig()
	contextLogger := log.WithContext(ctx)
	err := req.ParseMultipartForm(32 << 20)
	if err != nil {
		contextLogger.WithError(err).Error("Failed to parse request body")
		return err
	}

	file, _, err := req.FormFile("file")
	if err != nil {
		contextLogger.WithError(err).Error("Failed to get the file from request")
		return err
	}
	defer file.Close()

	buf := bytes.NewBuffer(nil)
	if _, err := io.Copy(buf, file); err != nil {
		contextLogger.WithError(err).Error("Failed to copy file contents to buffer")
		return err
	}

	excelFile, err := xlsx.OpenBinary(buf.Bytes())
	if err != nil {
		contextLogger.WithError(err).Error("Failed to convert bytes to excel file")
		return err
	}

	err = excelFile.Save(envValues.XlsFileLocation)
	if err != nil {
		contextLogger.WithError(err).Error("Failed to save excel file to disk")
		return err
	}
	return nil
}
