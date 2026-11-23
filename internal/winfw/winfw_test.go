package winfw

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestParseCommand(t *testing.T) {
	tests := []struct {
		args []string
		want Command
	}{
		{nil, Command{}},
		{[]string{}, Command{}},
		{[]string{"--setup-firewall"}, Command{Setup: true}},
		{[]string{"--setup-firewall", "--allow-public"}, Command{Setup: true, AllowPublic: true}},
		{[]string{"--allow-public", "--setup-firewall"}, Command{Setup: true, AllowPublic: true}},
		{[]string{"--allow-public"}, Command{}},
		{[]string{"--autostart"}, Command{}},
		{[]string{"--setup-firewall", "--bogus"}, Command{Setup: true, Invalid: true}},
		{[]string{"--setup-firewall", "extra"}, Command{Setup: true, Invalid: true}},
	}
	for _, tc := range tests {
		if got := ParseCommand(tc.args); got != tc.want {
			t.Errorf("ParseCommand(%q) = %+v, want %+v", tc.args, got, tc.want)
		}
	}
}

// recordRunner records the netsh commands and fails those it is told to.
type recordRunner struct {
	calls   [][]string
	failing func(args []string) bool
}

func (r *recordRunner) Run(args ...string) (string, error) {
	r.calls = append(r.calls, args)
	if r.failing != nil && r.failing(args) {
		return "No rules match the specified criteria.", errors.New("exit status 1")
	}
	return "Ok.", nil
}

func (r *recordRunner) joined() []string {
	out := make([]string, len(r.calls))
	for i, c := range r.calls {
		out[i] = strings.Join(c, " ")
	}
	return out
}

func TestSetupRemovesThenAddsTheRules(t *testing.T) {
	run := &recordRunner{}
	if err := Setup(run, `C:\App\openprintcast.exe`, 631, false); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"advfirewall firewall delete rule name=OpenPrintCast (IPP)",
		"advfirewall firewall delete rule name=OpenPrintCast (mDNS)",
		"advfirewall firewall delete rule name=OpenPrintCast (TCP)",
		"advfirewall firewall delete rule name=OpenPrintCast (UDP)",
		`advfirewall firewall delete rule name=all dir=in program=C:\App\openprintcast.exe`,
		"advfirewall firewall add rule name=OpenPrintCast (IPP) dir=in action=allow protocol=TCP localport=631 remoteip=localsubnet profile=domain,private description=OpenPrintCast local printing",
		"advfirewall firewall add rule name=OpenPrintCast (mDNS) dir=in action=allow protocol=UDP localport=5353 remoteip=localsubnet profile=domain,private description=OpenPrintCast local printing",
	}
	if got := run.joined(); !reflect.DeepEqual(got, want) {
		t.Fatalf("commands:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestSetupAddsPublicOnlyWhenAsked(t *testing.T) {
	for _, allow := range []bool{false, true} {
		run := &recordRunner{}
		_ = Setup(run, "", 631, allow)
		has := false
		for _, c := range run.joined() {
			if strings.Contains(c, "public") {
				has = true
			}
			if strings.Contains(c, "program=") {
				t.Errorf("a program rule removal without a program: %s", c)
			}
		}
		if has != allow {
			t.Errorf("allowPublic=%v but public in commands: %v", allow, has)
		}
	}
}

func TestSetupNeverTouchesTheInstallersRules(t *testing.T) {
	run := &recordRunner{}
	_ = Setup(run, "", 631, true)
	for _, c := range run.joined() {
		if strings.Contains(c, "OpenPrintCast - ") {
			t.Errorf("touches an installer rule: %s", c)
		}
	}
}

func TestSetupIgnoresMissingRulesButReportsFailedAdds(t *testing.T) {
	deletes := &recordRunner{failing: func(a []string) bool { return a[2] == "delete" }}
	if err := Setup(deletes, "x.exe", 631, false); err != nil {
		t.Fatalf("a delete that matched nothing failed the setup: %v", err)
	}
	adds := &recordRunner{failing: func(a []string) bool { return a[2] == "add" && strings.Contains(a[4], "IPP") }}
	err := Setup(adds, "x.exe", 631, false)
	if err == nil || !strings.Contains(err.Error(), "IPP") || strings.Contains(err.Error(), "mDNS") {
		t.Fatalf("err = %v, want only the IPP rule reported", err)
	}
	if len(adds.calls) != 7 {
		t.Fatalf("the mDNS rule was not tried after the IPP rule failed: %d calls", len(adds.calls))
	}
}

func TestElevatedArgs(t *testing.T) {
	if got := elevatedArgs(false); got != "--setup-firewall" {
		t.Errorf("args = %q", got)
	}
	if got := elevatedArgs(true); got != "--setup-firewall --allow-public" {
		t.Errorf("args = %q", got)
	}
	// What the elevated copy is started with must be what it understands.
	if cmd := ParseCommand(strings.Fields(elevatedArgs(true))); !cmd.Setup || !cmd.AllowPublic || cmd.Invalid {
		t.Errorf("the elevated copy would not understand %+v", cmd)
	}
}
