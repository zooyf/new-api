package main

type mediaURL struct {
	URL string `json:"url,omitempty"`
}

type volcengineContentItem struct {
	Type     string    `json:"type,omitempty"`
	Text     string    `json:"text,omitempty"`
	ImageURL *mediaURL `json:"image_url,omitempty"`
	VideoURL *mediaURL `json:"video_url,omitempty"`
	AudioURL *mediaURL `json:"audio_url,omitempty"`
	Role     string    `json:"role,omitempty"`
}

type volcengineTool struct {
	Type string `json:"type,omitempty"`
}

type volcengineSubmitRequest struct {
	Model                 string                  `json:"model"`
	Content               []volcengineContentItem `json:"content,omitempty"`
	CallbackURL           string                  `json:"callback_url,omitempty"`
	ReturnLastFrame       *bool                   `json:"return_last_frame,omitempty"`
	ServiceTier           string                  `json:"service_tier,omitempty"`
	ExecutionExpiresAfter *int                    `json:"execution_expires_after,omitempty"`
	GenerateAudio         *bool                   `json:"generate_audio,omitempty"`
	Draft                 *bool                   `json:"draft,omitempty"`
	Tools                 []volcengineTool        `json:"tools,omitempty"`
	SafetyIdentifier      string                  `json:"safety_identifier,omitempty"`
	Priority              *int                    `json:"priority,omitempty"`
	Resolution            string                  `json:"resolution,omitempty"`
	Ratio                 string                  `json:"ratio,omitempty"`
	Duration              *int                    `json:"duration,omitempty"`
	Frames                *int                    `json:"frames,omitempty"`
	Seed                  *int                    `json:"seed,omitempty"`
	CameraFixed           *bool                   `json:"camera_fixed,omitempty"`
	Watermark             *bool                   `json:"watermark,omitempty"`
}

type newAPIVideoRequest struct {
	Model    string         `json:"model"`
	Prompt   string         `json:"prompt"`
	Images   []string       `json:"images,omitempty"`
	Duration int            `json:"duration,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

type volcengineSubmitResponse struct {
	ID string `json:"id"`
}

type volcengineTaskResponse struct {
	ID              string                `json:"id"`
	Model           string                `json:"model,omitempty"`
	Status          string                `json:"status"`
	Content         volcengineTaskContent `json:"content"`
	Seed            int                   `json:"seed,omitempty"`
	Resolution      string                `json:"resolution,omitempty"`
	Duration        int                   `json:"duration,omitempty"`
	Ratio           string                `json:"ratio,omitempty"`
	FramesPerSecond int                   `json:"framespersecond,omitempty"`
	ServiceTier     string                `json:"service_tier,omitempty"`
	Tools           []volcengineTool      `json:"tools,omitempty"`
	Usage           volcengineTaskUsage   `json:"usage,omitempty"`
	Error           *volcengineTaskError  `json:"error,omitempty"`
	CreatedAt       int64                 `json:"created_at,omitempty"`
	UpdatedAt       int64                 `json:"updated_at,omitempty"`
}

type volcengineTaskContent struct {
	VideoURL string `json:"video_url,omitempty"`
}

type volcengineTaskUsage struct {
	CompletionTokens int                         `json:"completion_tokens,omitempty"`
	TotalTokens      int                         `json:"total_tokens,omitempty"`
	ToolUsage        volcengineTaskUsageToolData `json:"tool_usage,omitempty"`
}

type volcengineTaskUsageToolData struct {
	WebSearch int `json:"web_search,omitempty"`
}

type volcengineTaskError struct {
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}
