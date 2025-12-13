class MicCaptureProcessor extends AudioWorkletProcessor {
  constructor() {
    super();
    this.buffer = [];
    this.sampleRate = 16000; // target
    this.accumulated = [];
  }

  // resample to 16k by simple downsample (assumes input <= 48k)
  downsample(input, inRate, outRate) {
    if (inRate === outRate) return input;
    const ratio = inRate / outRate;
    const outLength = Math.floor(input.length / ratio);
    const out = new Int16Array(outLength);
    for (let i = 0; i < outLength; i++) {
      const idx = Math.floor(i * ratio);
      const sample = input[idx] * 0x7fff;
      out[i] = Math.max(-32768, Math.min(32767, sample));
    }
    return out;
  }

  process(inputs, outputs, parameters) {
    const input = inputs[0];
    if (!input || input.length === 0) return true;
    const channelData = input[0]; // mono
    const inRate = sampleRate; // AudioWorklet global
    const pcm16 = this.downsample(channelData, inRate, this.sampleRate);
    // accumulate ~300ms chunks: 16kHz * 0.3s = 4800 samples
    this.accumulated.push(pcm16);
    const total = this.accumulated.reduce((acc, a) => acc + a.length, 0);
    if (total >= 4800) {
      const merged = new Int16Array(total);
      let offset = 0;
      for (const part of this.accumulated) {
        merged.set(part, offset);
        offset += part.length;
      }
      this.accumulated = [];
      const b64 = this.toBase64(merged);
      this.port.postMessage({ type: 'audio.append', audio: b64 });
    }
    return true;
  }

  toBase64(int16Arr) {
    const buf = new Uint8Array(int16Arr.buffer);
    let binary = '';
    for (let i = 0; i < buf.byteLength; i++) {
      binary += String.fromCharCode(buf[i]);
    }
    return btoa(binary);
  }
}

registerProcessor('mic-capture', MicCaptureProcessor);
