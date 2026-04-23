package telegram

// OutboundMessage is the SQS payload produced by botworker and consumed by
// the outbound Lambda. Either Text (with optional Keyboard) or
// CallbackQueryID-only ack — usually both for callback handlers.
type OutboundMessage struct {
	ChatID          int64                 `json:"chat_id"`
	Text            string                `json:"text"`
	ParseMode       string                `json:"parse_mode,omitempty"`
	Keyboard        *InlineKeyboardMarkup `json:"keyboard,omitempty"`
	CallbackQueryID string                `json:"callback_query_id,omitempty"`
	CallbackText    string                `json:"callback_text,omitempty"`
}
