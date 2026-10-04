package config

import "testing"

func TestBackendValidate(t *testing.T) {
	cases := map[string]Backend{
		"no jar":     {Enabled: true},
		"not a jar":  {Enabled: true, Jar: "server.sh"},
		"escapes":    {Enabled: true, Jar: "../x.jar"},
		"port clash": {Enabled: true, Jar: "a.jar", WSPort: DefaultPort},
	}
	for name, b := range cases {
		c := Default()
		c.Backend = b
		if c.Validate() == nil {
			t.Errorf("%s: expected error", name)
		}
	}
	c := Default()
	c.Backend = Backend{Enabled: true, Jar: "fabric-server-launch.jar"}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
}
