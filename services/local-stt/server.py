"""呼びかけ判定用のローカルSTT。音声・文字起こしは保存しない。"""
import io
import json
import os
import threading
import wave
from http.server import BaseHTTPRequestHandler, HTTPServer

MAX_BYTES = 96_000 * 2 * 30 + 44
# Silero VADが発話とみなす確率の下限。faster-whisperの既定は0.5で、小さい声の呼びかけが
# 認識の前に捨てられている可能性があるため下げている。LOCAL_STT_VAD_THRESHOLDで変えられる。
DEFAULT_VAD_THRESHOLD = 0.35


def vad_threshold_from_env(value):
    threshold = float(value) if value else DEFAULT_VAD_THRESHOLD
    if not 0 < threshold < 1:
        raise ValueError('LOCAL_STT_VAD_THRESHOLDは0より大きく1未満にしてください')
    return threshold


def transcribe(model, audio, vad_threshold=DEFAULT_VAD_THRESHOLD):
    with wave.open(io.BytesIO(audio)) as wav:
        if wav.getnchannels() != 1 or wav.getsampwidth() != 2:
            raise ValueError('モノラル16bit PCM WAVが必要です')
        if not 8_000 <= wav.getframerate() <= 96_000 or wav.getnframes() / wav.getframerate() > 30:
            raise ValueError('音声は30秒以内にしてください')
    segments, _ = model.transcribe(
        io.BytesIO(audio), language='ja', beam_size=5,
        word_timestamps=True, vad_filter=True, condition_on_previous_text=False,
        # dictで渡すとfaster-whisperが内部で使う既定（無音160ms）が消えるため、明示する。
        vad_parameters={'threshold': vad_threshold, 'min_silence_duration_ms': 160},
    )
    texts, scores, details, words = [], [], [], []
    for segment in segments:
        details.append({'text': segment.text, 'avg_logprob': segment.avg_logprob,
                        'no_speech_prob': segment.no_speech_prob})
        if segment.no_speech_prob > 0.6:
            continue
        texts.append(segment.text)
        for word in segment.words or []:
            scores.append(word.probability)
            words.append({'word': word.word, 'probability': word.probability})
    # 認識モデルの単語確率の平均。校正された正答率ではない。
    return {'text': ''.join(texts).strip(), 'confidence': sum(scores) / len(scores) if scores else 0,
            'segments': details, 'words': words}


def handler_for(model, vad_threshold=DEFAULT_VAD_THRESHOLD):
    lock = threading.Lock()

    class Handler(BaseHTTPRequestHandler):
        def log_message(self, *_):
            pass

        def send_json(self, status, body):
            data = json.dumps(body, ensure_ascii=False, allow_nan=False).encode()
            self.send_response(status)
            self.send_header('Content-Type', 'application/json; charset=utf-8')
            self.send_header('Content-Length', str(len(data)))
            self.end_headers()
            self.wfile.write(data)

        def do_GET(self):
            self.send_json(200 if self.path == '/health' else 404, {'ready': self.path == '/health'})

        def do_POST(self):
            if self.path != '/transcribe':
                self.send_json(404, {'error': '未対応のパスです'})
                return
            try:
                length = int(self.headers.get('Content-Length', '0'))
                if not 44 <= length <= MAX_BYTES:
                    self.send_json(413, {'error': '音声サイズが不正です'})
                    return
                if self.headers.get('Content-Type') != 'audio/wav':
                    self.send_json(415, {'error': 'audio/wavが必要です'})
                    return
                audio = self.rfile.read(length)
                with lock:
                    result = transcribe(model, audio, vad_threshold)
                self.send_json(200, result)
            except (ValueError, wave.Error, EOFError):
                self.send_json(400, {'error': '音声形式が不正です'})
            except Exception as exc:
                print("ローカルSTT認識失敗:", type(exc).__name__, flush=True)
                self.send_json(503, {'error': 'ローカルSTTの認識に失敗しました'})

    return Handler


if __name__ == '__main__':
    from faster_whisper import WhisperModel
    vad_threshold = vad_threshold_from_env(os.getenv('LOCAL_STT_VAD_THRESHOLD'))
    model = WhisperModel(os.getenv('LOCAL_STT_MODEL', 'small'), device='cpu', compute_type='int8',
                         cpu_threads=int(os.getenv('LOCAL_STT_THREADS', '4')),
                         download_root='/models')
    HTTPServer(('0.0.0.0', 8000), handler_for(model, vad_threshold)).serve_forever()
