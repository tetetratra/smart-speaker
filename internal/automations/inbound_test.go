package automations

import "testing"

func TestDeliveryInboundMessageKeepsSourceAndText(t *testing.T) {
	message := (Delivery{ID: "job-1", Name: "室温", Instruction: "10分後に室温を知らせて"}).InboundMessage()
	want := "[アプリの予約ジョブ: 室温]\n予定時刻になりました。次の指示を今すぐ実行してください。指示に時刻や「〜後に」などの表現が含まれていても、それは予約時の表現なので、新しい予約の登録や予約内容の復唱はしないでください。\n指示: 10分後に室温を知らせて"
	if message.Source != "automation/job-1" || message.Text != want {
		t.Fatalf("出どころか本文が変わりました: %+v", message)
	}
}
