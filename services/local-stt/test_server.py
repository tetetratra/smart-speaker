import io
import unittest
import wave
from types import SimpleNamespace
from server import DEFAULT_VAD_THRESHOLD, transcribe, vad_threshold_from_env


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
        self.options = options
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

    def test_vad_parameters(self):
        model = Model()
        transcribe(model, wav(), vad_threshold=0.2)
        self.assertTrue(model.options['vad_filter'])
        self.assertEqual(model.options['vad_parameters'], {'threshold': 0.2, 'min_silence_duration_ms': 160})

    def test_vad_threshold_from_env(self):
        self.assertEqual(vad_threshold_from_env(None), DEFAULT_VAD_THRESHOLD)
        self.assertEqual(vad_threshold_from_env(''), DEFAULT_VAD_THRESHOLD)
        self.assertEqual(vad_threshold_from_env('0.25'), 0.25)
        for value in ['0', '1', '-0.1', 'abc']:
            with self.assertRaises(ValueError):
                vad_threshold_from_env(value)
