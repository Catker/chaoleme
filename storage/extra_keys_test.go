package storage

import "testing"

func TestExtraConstructorsUseStableKeys(t *testing.T) {
	t.Parallel()

	randomIO := NewRandomIOExtra(1.2, 0.8, true, false)
	if randomIO[ExtraWriteLatencyMS] != 1.2 || randomIO[ExtraReadLatencyMS] != 0.8 {
		t.Fatalf("随机 IO extra 不符合预期: %+v", randomIO)
	}
	if randomIO[ExtraDirectIOWrite] != true || randomIO[ExtraDirectIORead] != false {
		t.Fatalf("DirectIO extra 不符合预期: %+v", randomIO)
	}

	host := NewHostContextExtra(true, false, "kvm", true)
	if host[ExtraVirtualizationType] != "kvm" || host[ExtraStealDirectlyInterpretable] != true {
		t.Fatalf("运行环境 extra 不符合预期: %+v", host)
	}
}
