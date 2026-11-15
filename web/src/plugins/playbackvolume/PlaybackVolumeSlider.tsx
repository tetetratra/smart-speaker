import { playbackVolumeLevels, type PlaybackVolumeLevel } from './playbackVolume'

// 全端末で共通の再生音量を切り替えるスライダー。
export function PlaybackVolumeSlider({ level, onChange }: { level: PlaybackVolumeLevel; onChange: (level: PlaybackVolumeLevel) => void }) {
  return (
    <div className="live-playback-slider-row">
      <span className="live-playback-slider-label">再生音量</span>
      <input
        className="live-volume-slider"
        type="range"
        min={0}
        max={playbackVolumeLevels.length - 1}
        step={1}
        value={playbackVolumeLevels.indexOf(level)}
        aria-label="再生音量"
        onChange={(event) => onChange(playbackVolumeLevels[Number(event.currentTarget.value)])}
      />
    </div>
  )
}
