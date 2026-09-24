# typed: false
# This file is generated from published release checksums. Do not edit by hand.
# Tap: https://github.com/foae/kaneo-cli
class KaneoCli < Formula
  desc "Command-line client for Kaneo"
  homepage "https://github.com/foae/kaneo-cli"
  version "1.6.1"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/foae/kaneo-cli/releases/download/v1.6.1/kaneo-cli_1.6.1_darwin_arm64.tar.gz"
      sha256 "0aac5ee68adbcabf82c8aa22c802de403a46f31f92550c3e9f56f3fa25c95be4"
    else
      url "https://github.com/foae/kaneo-cli/releases/download/v1.6.1/kaneo-cli_1.6.1_darwin_amd64.tar.gz"
      sha256 "ea053959702d88743c96cf4bff3141fbbbb42dddc562835bb9b3466720951480"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/foae/kaneo-cli/releases/download/v1.6.1/kaneo-cli_1.6.1_linux_arm64.tar.gz"
      sha256 "8409d4b0f8c50135dd0fc987f0a7d15c8a4898a43299c0dec09792e36f0459f5"
    else
      url "https://github.com/foae/kaneo-cli/releases/download/v1.6.1/kaneo-cli_1.6.1_linux_amd64.tar.gz"
      sha256 "df4551ad03d4cb30f814682d9989b963e9cbb5cf6f6d3e4fe6d1740730f1869f"
    end
  end

  def install
    bin.install "kaneo-cli"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/kaneo-cli version")
  end
end
