# typed: false
# This file is generated from published release checksums. Do not edit by hand.
# Tap: https://github.com/foae/kaneo-cli
class KaneoCli < Formula
  desc "Command-line client for Kaneo"
  homepage "https://github.com/foae/kaneo-cli"
  version "5.0.0"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/foae/kaneo-cli/releases/download/v5.0.0/kaneo-cli_5.0.0_darwin_arm64.tar.gz"
      sha256 "8bc8b76aa26b828f069e377557363751471febee26263d0944753073d2c9a197"
    else
      url "https://github.com/foae/kaneo-cli/releases/download/v5.0.0/kaneo-cli_5.0.0_darwin_amd64.tar.gz"
      sha256 "7cc3fa6040ddc844bdfa33c60fb6bbfc09ac8c3b70d2d12b2be169eb6ca7e298"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/foae/kaneo-cli/releases/download/v5.0.0/kaneo-cli_5.0.0_linux_arm64.tar.gz"
      sha256 "040e7844f9c632990532f538cae827388c47790418546066131d8c5f2d118249"
    else
      url "https://github.com/foae/kaneo-cli/releases/download/v5.0.0/kaneo-cli_5.0.0_linux_amd64.tar.gz"
      sha256 "59b58f30e54b9bda1aa1170f868dc95ad053fa759be558a4385dda8b0ad7b2a9"
    end
  end

  def install
    bin.install "kaneo-cli"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/kaneo-cli version")
  end
end
