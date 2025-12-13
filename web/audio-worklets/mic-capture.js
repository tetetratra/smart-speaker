class MicCaptureProcessor extends AudioWorkletProcessor {
  constructor() {
    super();
    this.buffer = [];
    this.sampleRate = 24000; // target sample rate to match Realtime API output
    this.accumulated = [];
  }

  // resample to target rate by simple downsample (assumes input <= 48k)
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
    // accumulate ~300ms chunks: 24kHz * 0.3s = 7200 samples
    this.accumulated.push(pcm16);
    const total = this.accumulated.reduce((acc, a) => acc + a.length, 0);
    if (total >= 7200) {
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
    const chars = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/';
    let out = '';
    let i = 0;
    for (; i + 2 < buf.length; i += 3) {
      const n = (buf[i] << 16) | (buf[i + 1] << 8) | buf[i + 2];
      out += chars[(n >> 18) & 63];
      out += chars[(n >> 12) & 63];
      out += chars[(n >> 6) & 63];
      out += chars[n & 63];
    }
    const remaining = buf.length - i;
    if (remaining === 1) {
      const n = buf[i] << 16;
      out += chars[(n >> 18) & 63];
      out += chars[(n >> 12) & 63];
      out += '==';
    } else if (remaining === 2) {
      const n = (buf[i] << 16) | (buf[i + 1] << 8);
      out += chars[(n >> 18) & 63];
      out += chars[(n >> 12) & 63];
      out += chars[(n >> 6) & 63];
      out += '=';
    }
    return out;
  }
}

registerProcessor('mic-capture', MicCaptureProcessor);
