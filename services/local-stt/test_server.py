import io
import unittest
import wave
from types import SimpleNamespace
from server import transcribe


def wav(channels=1):
    out = io.BytesIO()
    with wave.open(out, 'wb') as f:
        f.setnchannels(channels)
        f.setsampwidth(2)
        f.setframerate(16000)
        f.writeframes(bytes(32000))
    return out.getvalue()


class Model:
    def transcribe(self, audio, **options):
        assert options['word_timestamps'] and options['language'] == 'ja'
        assert not options['condition_on_previous_text']
        return iter([
            SimpleNamespace(text='スピーカー', avg_logprob=-0.2, no_speech_prob=0.1,
                            words=[SimpleNamespace(word='スピーカー', probability=0.9)]),
            SimpleNamespace(text='幻覚', avg_logprob=-0.3, no_speech_prob=0.9, words=[]),
        ]), None


class TestLocalSTT(unittest.TestCase):
    def test_scores_and_no_speech(self):
        result = transcribe(Model(), wav())
        self.assertEqual(result['text'], 'スピーカー')
        self.assertEqual(result['confidence'], 0.9)
        self.assertEqual(result['words'][0]['probability'], 0.9)
        self.assertEqual(len(result['segments']), 2)

    def test_invalid_audio(self):
        with self.assertRaises(ValueError):
            transcribe(Model(), wav(channels=2))
