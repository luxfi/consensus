package config

type WaveConfig struct {
	Enable              bool
	VoteLimitPerBlock   int
	ExecuteOwned        bool
	ExecuteMixedOnFinal bool
	EpochFence          bool
	VotePrefix          []byte
}

func DefaultWave() WaveConfig {
	return WaveConfig{
		Enable:              true,
		VoteLimitPerBlock:   256,
		ExecuteOwned:        true,
		ExecuteMixedOnFinal: true,
		EpochFence:          true,
		VotePrefix:          []byte("WAVE/V1"),
	}
}
