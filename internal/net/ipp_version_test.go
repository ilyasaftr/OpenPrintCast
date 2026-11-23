package net

import (
	"testing"

	"github.com/OpenPrinting/goipp"
)

func TestNegotiateVersion(t *testing.T) {
	v10 := goipp.MakeVersion(1, 0)
	v11 := goipp.MakeVersion(1, 1)
	v20 := goipp.MakeVersion(2, 0)
	for _, tc := range []struct {
		client          goipp.Version
		wantServe, want goipp.Version
	}{
		{v10, v11, v11},
		{v11, v11, v11},
		{v20, v20, v20},
		{goipp.DefaultVersion, v20, v20},
		{goipp.MakeVersion(2, 1), v20, v20},
		{goipp.MakeVersion(0, 0), v11, v11},
		{goipp.MakeVersion(3, 0), v11, v11},
	} {
		serve, echo := NegotiateVersion(tc.client)
		if serve != tc.wantServe || echo != tc.want {
			t.Errorf("NegotiateVersion(%s) = serve %s echo %s, want serve %s echo %s",
				tc.client, serve, echo, tc.wantServe, tc.want)
		}
	}
}

func TestIPP20RequiresKnownMandatoryCapabilities(t *testing.T) {
	shared := testShared()
	shared.SetSpooler(&unknownDefaultMediaSpooler{&mediaSpooler{&fakeSpooler{}}})
	printer := testPrinter()
	req := goipp.NewRequest(ipp11, goipp.OpGetPrinterAttributes, 1)
	rsp := serveIPP(req, nil, printer, shared, "pc", 631)
	if got := attrValues(t, rsp.Printer, "ipp-versions-supported"); len(got) != 1 || got[0] != "1.1" {
		t.Fatalf("versions with unknown media default = %v", got)
	}
	req.Version = ipp20
	rsp = serveIPP(req, nil, printer, shared, "pc", 631)
	if goipp.Status(rsp.Code) != goipp.StatusErrorVersionNotSupported || rsp.Version != ipp11 {
		t.Fatalf("unsupported 2.0: status=%s version=%s", goipp.Status(rsp.Code), rsp.Version)
	}
}

func TestIPP20AdvertisedAndMinorVersionNegotiated(t *testing.T) {
	shared := testShared()
	printer := testPrinter()
	req := goipp.NewRequest(ipp11, goipp.OpGetPrinterAttributes, 1)
	rsp := serveIPP(req, nil, printer, shared, "pc", 631)
	if got := attrValues(t, rsp.Printer, "ipp-versions-supported"); len(got) != 2 || got[0] != "1.1" || got[1] != "2.0" {
		t.Fatalf("versions = %v", got)
	}
	req.Version = goipp.MakeVersion(2, 1)
	rsp = serveIPP(req, nil, printer, shared, "pc", 631)
	if goipp.Status(rsp.Code) != goipp.StatusOk || rsp.Version != ipp20 {
		t.Fatalf("2.1 negotiation: status=%s version=%s", goipp.Status(rsp.Code), rsp.Version)
	}
	req.Version = goipp.MakeVersion(3, 0)
	rsp = serveIPP(req, nil, printer, shared, "pc", 631)
	if goipp.Status(rsp.Code) != goipp.StatusErrorVersionNotSupported || rsp.Version != ipp20 {
		t.Fatalf("3.0 negotiation: status=%s version=%s", goipp.Status(rsp.Code), rsp.Version)
	}
}

func TestVersionEchoInResponse(t *testing.T) {
	shared := testShared()
	for _, client := range []goipp.Version{goipp.MakeVersion(1, 1), goipp.MakeVersion(2, 0)} {
		req := goipp.NewRequest(client, goipp.OpGetPrinterAttributes, 7)
		req.Operation.Add(goipp.MakeAttr("attributes-charset",
			goipp.TagCharset, goipp.String("utf-8")))
		rsp := serveIPP(req, nil, testPrinter(), shared, "pc", 631)
		if rsp.Version != client {
			t.Errorf("client %s: response version %s, want echo %s", client, rsp.Version, client)
		}
		if goipp.Status(rsp.Code) != goipp.StatusOk {
			t.Errorf("client %s: status %s, want successful-ok", client, goipp.Status(rsp.Code))
		}
		if got := attrValues(t, rsp.Printer, "printer-info"); len(got) != 1 {
			t.Errorf("client %s: markers missing from GPA response", client)
		}
	}
}
