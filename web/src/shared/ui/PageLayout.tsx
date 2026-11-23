import React from 'react'
import './PageLayout.css'

// メイン画面以外の画面の外枠。画面の高さに合わせた枠と、下の「app画面」ボタンだけを持ち、中身は各画面が描く。
// 中身の領域は、収まらなければこの枠の中でスクロールする。
export function PageLayout({ onBack, children }: { onBack: () => void; children: React.ReactNode }) {
  return (
    <div className="page-layout">
      <div className="page-layout-body">{children}</div>
      <button className="page-layout-back" onClick={onBack}>app画面</button>
    </div>
  )
}
