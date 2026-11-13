package types

type RelayFormat string

const (
	RelayFormatOpenAI                    RelayFormat = "openai"
	RelayFormatClaude                                = "claude"
	RelayFormatGemini                                = "gemini"
	RelayFormatOpenAIResponses                       = "openai_responses"
	RelayFormatOpenAIResponsesCompaction             = "openai_responses_compaction"
	RelayFormatOpenAIAlphaSearch                     = "openai_alpha_search"
	RelayFormatDeepSeekTools                         = "deepseek_tools"
	RelayFormatOpenAIAudio                           = "openai_audio"
	RelayFormatOpenAIImage                           = "openai_image"
	RelayFormatOpenAIRealtime                        = "openai_realtime"
	RelayFormatRerank                                = "rerank"
	RelayFormatEmbedding                             = "embedding"
	// RelayFormatSystemOne is the TypeSafe Jev decision model: a text under
	// judgement plus question definitions in, answers out. It is relayed
	// without protocol conversion.
	RelayFormatSystemOne = "systemone"

	RelayFormatTask    = "task"
	RelayFormatMjProxy = "mj_proxy"
)
