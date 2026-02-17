#!/usr/bin/env ruby
require "base64"
require "openssl"
require "securerandom"
require "shellwords"

token  = ENV.fetch("SWITCHBOT_TOKEN")
secret = ENV.fetch("SWITCHBOT_SECRET")

timestamp = (Time.now.to_f * 1000).to_i.to_s
nonce     = SecureRandom.hex(16)
payload   = token + timestamp + nonce

sign = Base64.strict_encode64(
  OpenSSL::HMAC.digest("SHA256", secret, payload)
)

cmd = [
  "curl", "-s",
  "-H", "Authorization: #{token}",
  "-H", "Content-Type: application/json; charset=utf8",
  "-H", "t: #{timestamp}",
  "-H", "nonce: #{nonce}",
  "-H", "sign: #{sign}",
  "https://api.switch-bot.com/v1.1/devices" # scenes を取得する場合は /v1.1/scenes
]
puts `#{cmd.shelljoin}`
