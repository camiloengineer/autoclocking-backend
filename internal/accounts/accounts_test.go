package accounts

import "testing"

func TestNewAccountNormalizesAndValidates(t *testing.T) {
	acc, err := NewAccount("  CGonzalez@Robotia.CL ", "robotia..", true)
	if err != nil {
		t.Fatalf("NewAccount: %v", err)
	}
	if acc.Email != "cgonzalez@robotia.cl" {
		t.Errorf("Email = %q, want normalized lowercase/trimmed", acc.Email)
	}
	if !acc.Active {
		t.Error("Active = false, want true")
	}

	if _, err := NewAccount("not-an-email", "x", true); err == nil {
		t.Error("expected error for invalid email")
	}
	if _, err := NewAccount("a@b.cl", "", true); err == nil {
		t.Error("expected error for empty password")
	}
}

func TestPasswordBase64RoundTrip(t *testing.T) {
	plain := "robotia..#áé"
	got, err := DecodePassword(EncodePassword(plain))
	if err != nil {
		t.Fatalf("DecodePassword: %v", err)
	}
	if got != plain {
		t.Errorf("round-trip = %q, want %q", got, plain)
	}
}

func TestRedactDropsSecret(t *testing.T) {
	acc := Account{Email: "cgonzalez@robotia.cl", Password: "robotia..", JobID: "2690", Active: true}
	view := acc.Redact()
	if view.Email != acc.Email || view.JobID != acc.JobID || view.Active != acc.Active {
		t.Errorf("Redact dropped a public field: %+v", view)
	}
}

func TestMask(t *testing.T) {
	cases := map[string]string{
		"cgonzalez@robotia.cl": "cg***@robotia.cl",
		"a@b.cl":               "a***@b.cl",
		"nope":                 "***",
	}
	for in, want := range cases {
		if got := Mask(in); got != want {
			t.Errorf("Mask(%q) = %q, want %q", in, got, want)
		}
	}
}
