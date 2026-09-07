package coverage

import (
	"context"
	"reflect"

	"github.com/Allan-Nava/checkfleet/internal/discovery"
	"github.com/Allan-Nava/checkfleet/internal/engine"
)

// Discovered resolves every module's discovery sources and returns the targets
// they add on top of the ones written in the config (CF-179).
//
// It exists so that `checkfleet targets` answers the same question a run would:
// with a catalog or SRV in play, the static config no longer lists what gets
// checked, and a coverage tool reading only the YAML would under-report exactly
// where discovery is doing the work.
//
// Errors are returned per module rather than aborting: a broken catalog is
// worth naming, but it must not hide the modules that resolved fine.
func Discovered(ctx context.Context, cfg *engine.Config) ([]Target, map[string]error) {
	if cfg == nil {
		return nil, nil
	}
	var out []Target
	var errs map[string]error

	checks := reflect.ValueOf(&cfg.Checks).Elem()
	ct := checks.Type()
	for i := 0; i < checks.NumField(); i++ {
		f := checks.Field(i)
		if f.Kind() != reflect.Pointer || f.IsNil() {
			continue
		}
		df := f.Elem().FieldByName("Discovery")
		if !df.IsValid() || df.Type() != reflect.TypeOf(engine.Discovery{}) {
			continue
		}
		d, _ := df.Interface().(engine.Discovery)
		if !d.Set() {
			continue
		}
		module := yamlName(ct.Field(i))
		hosts, err := discovery.Resolve(ctx, d)
		if err != nil {
			if errs == nil {
				errs = map[string]error{}
			}
			errs[module] = err
		}
		for _, h := range hosts {
			out = append(out, Target{
				Module: module, Name: h.Name, Hosts: hostsOf(h.Address),
				Port: portOf(h.Address), Source: h.Source,
			})
		}
	}
	return out, errs
}
