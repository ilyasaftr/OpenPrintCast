package net

import (
	"slices"
	"strings"
	"testing"

	"github.com/OpenPrinting/goipp"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

// supplySpooler is a driver whose printer reports supplies, like a network
// printer whose Windows port has SNMP switched on.
type supplySpooler struct {
	*mediaSpooler
	supplies []core.Supply
	host     string
	duplex   bool
}

func (s *supplySpooler) SupplyLevels(string) ([]core.Supply, string) { return s.supplies, s.host }

func (s *supplySpooler) DuplexSupported(string) bool { return s.duplex }

// cmykSupplies is a colour laser printer: toner at various levels and a
// waste bin that is 20% full.
func cmykSupplies() []core.Supply {
	return []core.Supply{
		{Name: "Black Toner Cartridge", Color: "black", Kind: core.SupplyToner, Level: 8},
		{Name: "Cyan Toner Cartridge", Color: "cyan", Kind: core.SupplyToner, Level: 75},
		{Name: "Magenta Toner Cartridge", Color: "magenta", Kind: core.SupplyToner, Level: 40},
		{Name: "Yellow Toner Cartridge", Color: "yellow", Kind: core.SupplyToner, Level: 90},
		{Name: "Waste Toner Bin", Kind: core.SupplyWasteToner, Level: 20, Receptacle: true},
	}
}

func newSupplySpooler(supplies []core.Supply, host string) *supplySpooler {
	return &supplySpooler{mediaSpooler: &mediaSpooler{&fakeSpooler{}}, supplies: supplies, host: host}
}

// printerAttributes serves Get-Printer-Attributes over the wire encoding.
func printerAttributes(t *testing.T, spooler Spooler) goipp.Attributes {
	t.Helper()
	shared := testShared()
	if spooler != nil {
		shared.SetSpooler(spooler)
	}
	rsp := serveIPP(goipp.NewRequest(ipp20, goipp.OpGetPrinterAttributes, 1), nil, testPrinter(), shared, "pc", 631)
	wire, err := rsp.EncodeBytes()
	if err != nil {
		t.Fatal(err)
	}
	var decoded goipp.Message
	if err := decoded.DecodeBytes(wire); err != nil {
		t.Fatal(err)
	}
	return decoded.Printer
}

// supplyAttributeNames are every attribute that exists only because
// supplies are known.
var supplyAttributeNames = []string{
	"marker-names", "marker-types", "marker-levels", "marker-colors", "marker-low-levels",
	"marker-high-levels", "marker-message", "printer-supply", "printer-supply-description",
	"printer-supply-info-uri",
}

// hasAttr reports whether attrs holds name.
func hasAttr(attrs goipp.Attributes, name string) bool { return findAttr(attrs, name) != nil }

func TestSupplyAttributesForCMYKToner(t *testing.T) {
	attrs := printerAttributes(t, newSupplySpooler(cmykSupplies(), "192.168.1.50"))
	for name, want := range map[string][]string{
		"marker-names":               {"Black Toner Cartridge", "Cyan Toner Cartridge", "Magenta Toner Cartridge", "Yellow Toner Cartridge", "Waste Toner Bin"},
		"marker-types":               {"toner", "toner", "toner", "toner", "waste-toner"},
		"marker-levels":              {"8", "75", "40", "90", "20"},
		"marker-colors":              {"#000000", "#00FFFF", "#FF00FF", "#FFFF00", "none"},
		"marker-low-levels":          {"10", "10", "10", "10", "0"},
		"marker-high-levels":         {"100", "100", "100", "100", "100"},
		"marker-message":             {"Black 8%, Cyan 75%, Magenta 40%, Yellow 90%, Waste Toner Bin 20%"},
		"printer-supply-description": {"Black Toner Cartridge", "Cyan Toner Cartridge", "Magenta Toner Cartridge", "Yellow Toner Cartridge", "Waste Toner Bin"},
		"printer-supply-info-uri":    {"http://192.168.1.50/"},
	} {
		if got := attrValues(t, attrs, name); !slices.Equal(got, want) {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
}

func TestPrinterSupplyUsesPWGSyntaxAndOctetStrings(t *testing.T) {
	attrs := printerAttributes(t, newSupplySpooler(cmykSupplies(), ""))
	supply := findAttr(attrs, "printer-supply")
	if supply == nil {
		t.Fatal("printer-supply missing")
	}
	want := []string{
		"index=1;class=supplyThatIsConsumed;type=toner;unit=percent;maxcapacity=100;level=8;colorantname=black;",
		"index=2;class=supplyThatIsConsumed;type=toner;unit=percent;maxcapacity=100;level=75;colorantname=cyan;",
		"index=3;class=supplyThatIsConsumed;type=toner;unit=percent;maxcapacity=100;level=40;colorantname=magenta;",
		"index=4;class=supplyThatIsConsumed;type=toner;unit=percent;maxcapacity=100;level=90;colorantname=yellow;",
		"index=5;class=receptacleThatIsFilled;type=wasteToner;unit=percent;maxcapacity=100;level=20;colorantname=no-color;",
	}
	if len(supply.Values) != len(want) {
		t.Fatalf("%d values, want %d", len(supply.Values), len(want))
	}
	for i, value := range supply.Values {
		binary, isBinary := value.V.(goipp.Binary)
		if value.T != goipp.TagString || !isBinary || string(binary) != want[i] {
			t.Errorf("value %d = %v %q, want octetString %q", i+1, value.T, value.V, want[i])
		}
	}
	// The description attribute is plain text, one value per supply.
	description := findAttr(attrs, "printer-supply-description")
	if description == nil || len(description.Values) != len(want) || description.Values[0].T != goipp.TagText {
		t.Errorf("printer-supply-description: %v", description)
	}
	if info := findAttr(attrs, "printer-supply-info-uri"); info == nil || len(info.Values) != 1 || info.Values[0].T != goipp.TagURI {
		t.Errorf("printer-supply-info-uri: %v", info)
	}
}

func TestMarkerAttributeSyntaxes(t *testing.T) {
	attrs := printerAttributes(t, newSupplySpooler(cmykSupplies(), ""))
	for name, tag := range map[string]goipp.Tag{
		"marker-names": goipp.TagName, "marker-types": goipp.TagKeyword, "marker-levels": goipp.TagInteger,
		"marker-colors": goipp.TagName, "marker-low-levels": goipp.TagInteger,
		"marker-high-levels": goipp.TagInteger, "marker-message": goipp.TagText,
	} {
		if attr := findAttr(attrs, name); attr == nil || len(attr.Values) == 0 || attr.Values[0].T != tag {
			t.Errorf("%s: %v, want syntax %v", name, attr, tag)
		}
	}
}

func TestSupplyAttributesAreOmittedWithoutData(t *testing.T) {
	for name, spooler := range map[string]Spooler{
		"backend without supplies":     &mediaSpooler{&fakeSpooler{}},
		"printer that reports nothing": newSupplySpooler(nil, "192.168.1.50"),
		"only unpublishable kinds":     newSupplySpooler([]core.Supply{{Name: "Transfer Kit", Kind: core.SupplyOther, Level: 50}}, "192.168.1.50"),
	} {
		t.Run(name, func(t *testing.T) {
			attrs := printerAttributes(t, spooler)
			for _, attr := range supplyAttributeNames {
				if hasAttr(attrs, attr) {
					t.Errorf("%s present without data", attr)
				}
			}
			if reasons := attrValues(t, attrs, "printer-state-reasons"); !slices.Equal(reasons, []string{"none"}) {
				t.Errorf("printer-state-reasons = %v", reasons)
			}
		})
	}
	// A description of a printer without any shared state, as in tests that
	// only describe a printer, has none either.
	rsp := printerAttrsResponse(goipp.NewRequest(ipp20, goipp.OpGetPrinterAttributes, 1), testPrinter(), nil, "pc", 631)
	if hasAttr(rsp.Printer, "printer-supply") {
		t.Error("printer-supply present without shared state")
	}
}

func TestSupplyLevelsThatAreNotNumbers(t *testing.T) {
	attrs := printerAttributes(t, newSupplySpooler([]core.Supply{
		{Name: "Black", Color: "black", Kind: core.SupplyInk, Level: core.SupplyLevelSomeRemaining},
		{Name: "Cyan", Color: "cyan", Kind: core.SupplyInk, Level: core.SupplyLevelUnknown},
	}, ""))
	if got := attrValues(t, attrs, "marker-levels"); !slices.Equal(got, []string{"-3", "-2"}) {
		t.Errorf("marker-levels = %v", got)
	}
	want := []string{
		"index=1;class=supplyThatIsConsumed;type=ink;unit=other;maxcapacity=-2;level=-3;colorantname=black;",
		"index=2;class=supplyThatIsConsumed;type=ink;unit=other;maxcapacity=-2;level=-2;colorantname=cyan;",
	}
	for i, value := range findAttr(attrs, "printer-supply").Values {
		if string(value.V.(goipp.Binary)) != want[i] {
			t.Errorf("printer-supply[%d] = %q, want %q", i, value.V, want[i])
		}
	}
	if hasAttr(attrs, "marker-message") {
		t.Error("marker-message present although no level is a number")
	}
	if reasons := attrValues(t, attrs, "printer-state-reasons"); !slices.Equal(reasons, []string{"none"}) {
		t.Errorf("levels that are not numbers changed the reasons: %v", reasons)
	}
}

// bridgePage stands for the bridge's own page of the printer, which is
// printer-more-info: the fallback of printer-supply-info-uri.
const bridgePage = "(printer-more-info)"

func TestSupplyInfoURI(t *testing.T) {
	for name, tc := range map[string]struct{ host, want string }{
		"printer's own address":  {"192.168.1.50", "http://192.168.1.50/"},
		"printer's own name":     {"office-printer.lan", "http://office-printer.lan/"},
		"IPv6 address":           {"2001:db8::7", "http://[2001:db8::7]/"},
		"link-local with a zone": {"fe80::1%eth0", bridgePage},
		"address unknown":        {"", bridgePage},
	} {
		t.Run(name, func(t *testing.T) {
			attrs := printerAttributes(t, newSupplySpooler(cmykSupplies(), tc.host))
			want := tc.want
			if want == bridgePage {
				want = attrValues(t, attrs, "printer-more-info")[0]
			}
			if got := attrValues(t, attrs, "printer-supply-info-uri"); !slices.Equal(got, []string{want}) {
				t.Fatalf("printer-supply-info-uri = %v, want %q", got, want)
			}
		})
	}
}

func TestSupplyReasonsFollowLevels(t *testing.T) {
	toner := func(level core.SupplyLevel) []core.Supply {
		return []core.Supply{{Name: "Black", Color: "black", Kind: core.SupplyToner, Level: level}}
	}
	for name, tc := range map[string]struct {
		supplies []core.Supply
		want     []string
	}{
		"plenty":        {toner(50), []string{"none"}},
		"low":           {toner(10), []string{"marker-supply-low-warning"}},
		"empty":         {toner(0), []string{"marker-supply-empty-error"}},
		"low and empty": {append(toner(5), core.Supply{Name: "Cyan", Kind: core.SupplyInk, Level: 0}), []string{"marker-supply-empty-error"}},
	} {
		t.Run(name, func(t *testing.T) {
			attrs := printerAttributes(t, newSupplySpooler(tc.supplies, ""))
			if got := attrValues(t, attrs, "printer-state-reasons"); !slices.Equal(got, tc.want) {
				t.Fatalf("printer-state-reasons = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestMergeSupplyReasons(t *testing.T) {
	low, empty := []string{"marker-supply-low-warning"}, []string{"marker-supply-empty-error"}
	for name, tc := range map[string]struct{ reasons, derived, want []string }{
		"nothing derived leaves the reasons":           {[]string{"none"}, nil, []string{"none"}},
		"derived replaces none":                        {[]string{"none"}, low, low},
		"added to other reasons":                       {[]string{"media-empty"}, empty, []string{"media-empty", "marker-supply-empty-error"}},
		"same condition is not repeated":               {low, low, low},
		"windows empty equals derived empty":           {[]string{"marker-supply-empty"}, empty, []string{"marker-supply-empty"}},
		"windows low equals derived low":               {[]string{"marker-supply-low"}, low, []string{"marker-supply-low"}},
		"empty beats the driver's low":                 {[]string{"media-low", "marker-supply-low-warning"}, empty, []string{"media-low", "marker-supply-empty-error"}},
		"derived low does not undo the driver's empty": {[]string{"marker-supply-empty"}, low, []string{"marker-supply-empty"}},
		"unrelated reasons stay":                       {[]string{"toner-low-report", "door-open"}, low, []string{"toner-low-report", "door-open", "marker-supply-low-warning"}},
	} {
		t.Run(name, func(t *testing.T) {
			input := slices.Clone(tc.reasons)
			if got := mergeSupplyReasons(tc.reasons, tc.derived); !slices.Equal(got, tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			if !slices.Equal(tc.reasons, input) {
				t.Fatalf("the driver's reasons were modified: %v", tc.reasons)
			}
		})
	}
}

// A driver that reports toner empty through Windows and the supplies at 0%
// say one thing, not two.
func TestSupplyReasonsMergeWithDriverStatus(t *testing.T) {
	base := newSupplySpooler([]core.Supply{{Name: "Black", Color: "black", Kind: core.SupplyToner, Level: 0}}, "")
	base.reportedState, base.reportedReasons = printerStateIdle, []string{"marker-supply-empty", "media-needed"}
	spooler := &statusSupplySpooler{supplySpooler: base}
	attrs := printerAttributes(t, spooler)
	want := []string{"marker-supply-empty", "media-needed"}
	if got := attrValues(t, attrs, "printer-state-reasons"); !slices.Equal(got, want) {
		t.Fatalf("printer-state-reasons = %v, want %v", got, want)
	}
}

// statusSupplySpooler adds the Windows status the fake spooler holds.
type statusSupplySpooler struct{ *supplySpooler }

func (s *statusSupplySpooler) IPPStatus(string) (int, []string, bool) {
	return s.reportedState, s.reportedReasons, true
}

func TestSupplyAttributesCanBeRequestedAlone(t *testing.T) {
	shared := testShared()
	shared.SetSpooler(newSupplySpooler(cmykSupplies(), ""))
	req := goipp.NewRequest(ipp20, goipp.OpGetPrinterAttributes, 1)
	req.Operation.Add(goipp.MakeAttr("requested-attributes", goipp.TagKeyword, goipp.String("marker-levels"), goipp.String("printer-supply")))
	rsp := serveIPP(req, nil, testPrinter(), shared, "pc", 631)
	var names []string
	for _, a := range rsp.Printer {
		names = append(names, a.Name)
	}
	slices.Sort(names)
	if want := []string{"marker-levels", "printer-supply"}; !slices.Equal(names, want) {
		t.Fatalf("attributes = %v, want %v", names, want)
	}
}

func TestSupplyDescriptionNeverContainsTheHostOrSecrets(t *testing.T) {
	attrs := printerAttributes(t, newSupplySpooler(cmykSupplies(), "192.168.1.50"))
	for _, a := range attrs {
		for _, v := range a.Values {
			if strings.Contains(strings.ToLower(v.V.String()), "community") {
				t.Fatalf("%s mentions a community: %v", a.Name, v.V)
			}
		}
	}
}
