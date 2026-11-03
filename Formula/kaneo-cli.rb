# typed: false
# This file is generated from published release checksums. Do not edit by hand.
# Tap: https://github.com/foae/kaneo-cli
class KaneoCli < Formula
  desc "Command-line client for Kaneo"
  homepage "https://github.com/foae/kaneo-cli"
  version "5.0.1"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/foae/kaneo-cli/releases/download/v5.0.1/kaneo-cli_5.0.1_darwin_arm64.tar.gz"
      sha256 "8504c8abb154b3203a8fb9e410467820bc1eecf210a1da796c94ead77a34fce4"
    else
      url "https://github.com/foae/kaneo-cli/releases/download/v5.0.1/kaneo-cli_5.0.1_darwin_amd64.tar.gz"
      sha256 "86d7fe06a570972e36c9f1ef32e8d94c4b882333481f17cf03abff1e16f79426"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/foae/kaneo-cli/releases/download/v5.0.1/kaneo-cli_5.0.1_linux_arm64.tar.gz"
      sha256 "d274eda38013795c65b9170d8edfd54c62b5aa640018b81b1e89bc6f36bcad81"
    else
      url "https://github.com/foae/kaneo-cli/releases/download/v5.0.1/kaneo-cli_5.0.1_linux_amd64.tar.gz"
      sha256 "0e4d1d2bdabf112c76d143077c385fc0c849fc8d9562e5c21cea77e6dc4daccb"
    end
  end

  def install
    bin.install "kaneo-cli"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/kaneo-cli version")
  end
end
