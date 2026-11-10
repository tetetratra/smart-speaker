package automations

import "github.com/tetetratra/smart-speaker/internal/types"

// InboundMessage は予約時の指示を、復唱や再予約をせず今実行する文章にする。
func (d Delivery) InboundMessage() types.InboundMessage {
	return types.InboundMessage{
		Source: "automation/" + d.ID,
		Text:   "[アプリの予約ジョブ: " + d.Name + "]\n予定時刻になりました。次の指示を今すぐ実行してください。指示に時刻や「〜後に」などの表現が含まれていても、それは予約時の表現なので、新しい予約の登録や予約内容の復唱はしないでください。\n指示: " + d.Instruction,
	}
}
