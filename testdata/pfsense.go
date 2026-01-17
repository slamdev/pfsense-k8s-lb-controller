package testdata

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/manager"
)

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
		srv.Start()
		<-ctx.Done()
		srv.Close()
		slog.InfoContext(ctx, "mock pfsense server stopped")
		return nil
	}
}

func xmlrpcHandler(w http.ResponseWriter, r *http.Request) {
	bytedata, _ := io.ReadAll(r.Body)
	body := string(bytedata)
	var response string
	if strings.Contains(body, "pfsense.host_firmware_version") {
		response = hostFirmwareVersionResponse
	} else {
		response = notFoundResponse
	}
	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "text/xml; charset=utf-8")
	_, _ = w.Write([]byte(response))
}

const notFoundResponse = `
<?xml version="1.0" encoding="UTF-8"?>
<methodResponse>
   <fault>
      <value>
         <struct>
            <member>
               <name>faultCode</name>
               <value>
                  <int>-32601</int>
               </value>
            </member>
            <member>
               <name>faultString</name>
               <value>
                  <string>server error. requested method not found</string>
               </value>
            </member>
         </struct>
      </value>
   </fault>
</methodResponse>
`

const hostFirmwareVersionResponse = `
<?xml version="1.0" encoding="UTF-8"?>
<methodResponse>
   <params>
      <param>
         <value>
            <struct>
               <member>
                  <name>firmware</name>
                  <value>
                     <struct>
                        <member>
                           <name>version</name>
                           <value>
                              <string>2.7.0-RELEASE</string>
                           </value>
                        </member>
                     </struct>
                  </value>
               </member>
               <member>
                  <name>kernel</name>
                  <value>
                     <struct>
                        <member>
                           <name>version</name>
                           <value>
                              <string>14.0</string>
                           </value>
                        </member>
                     </struct>
                  </value>
               </member>
               <member>
                  <name>base</name>
                  <value>
                     <struct>
                        <member>
                           <name>version</name>
                           <value>
                              <string>14.0</string>
                           </value>
                        </member>
                     </struct>
                  </value>
               </member>
               <member>
                  <name>platform</name>
                  <value>
                     <string>pfSense</string>
                  </value>
               </member>
               <member>
                  <name>config_version</name>
                  <value>
                     <string>22.9</string>
                  </value>
               </member>
            </struct>
         </value>
      </param>
   </params>
</methodResponse>
`
