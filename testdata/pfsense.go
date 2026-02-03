package testdata

import (
	"context"
	"embed"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/manager"
)

//go:embed *.xml
var PFSenseFS embed.FS

var hostFirmwareVersionResponse string
var acceptedResponse string
var notFoundResponse string
var backupConfigSectionResponse string

func MockPfsenseServer() (string, manager.RunnableFunc) {
	mux := http.NewServeMux()
	mux.HandleFunc("/xmlrpc.php", xmlrpcHandler)
	srv := httptest.NewUnstartedServer(mux)
	// httptest server binds to 127.0.0.1 so it is not accessible from docker containers
	// we need to bind to 0.0.0.0
	l, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		panic(err)
	}
	srv.Listener = l

	return "http://" + l.Addr().String(), func(ctx context.Context) error {
		backupConfigSectionResponse = loadResponse("backup_config_section.xml")
		hostFirmwareVersionResponse = loadResponse("host_firmware_version.xml")
		acceptedResponse = loadResponse("accepted.xml")
		notFoundResponse = loadResponse("not_found.xml")

		srv.Start()
		<-ctx.Done()
		srv.Close()
		slog.InfoContext(ctx, "mock pfsense server stopped")
		return nil
	}
}

func loadResponse(filename string) string {
	b, err := PFSenseFS.ReadFile(filename)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func xmlrpcHandler(w http.ResponseWriter, r *http.Request) {
	bytedata, _ := io.ReadAll(r.Body)
	body := string(bytedata)
	slog.InfoContext(r.Context(), "mock pfsense server received request", "body", body)
	var response string
	if strings.Contains(body, "pfsense.host_firmware_version") {
		response = hostFirmwareVersionResponse
	} else if strings.Contains(body, "pfsense.backup_config_section") {
		response = backupConfigSectionResponse
	} else if strings.Contains(body, "pfsense.restore_config_section") {
		response = acceptedResponse
	} else {
		response = notFoundResponse
	}
	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "text/xml; charset=utf-8")
	_, _ = w.Write([]byte(response))
}
