# typed: false
# This file is generated from published release checksums. Do not edit by hand.
# Tap: https://github.com/foae/kaneo-cli
class KaneoCli < Formula
  desc "Command-line client for Kaneo"
  homepage "https://github.com/foae/kaneo-cli"
  version "1.5.0"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/foae/kaneo-cli/releases/download/v1.5.0/kaneo-cli_1.5.0_darwin_arm64.tar.gz"
      sha256 "8c981e110779a3aca9baaf4824b7903e93e7e88a9d863cc073f444e66d68cf52"
    else
      url "https://github.com/foae/kaneo-cli/releases/download/v1.5.0/kaneo-cli_1.5.0_darwin_amd64.tar.gz"
      sha256 "9dc7f54edf463ba76af568c865a117355124b31d7029953c6522cf05ff50ce2b"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/foae/kaneo-cli/releases/download/v1.5.0/kaneo-cli_1.5.0_linux_arm64.tar.gz"
      sha256 "3cca36e3d7beb37c2053529d8466f76157d087fc42958910373c5cc47f90aaf2"
    else
      url "https://github.com/foae/kaneo-cli/releases/download/v1.5.0/kaneo-cli_1.5.0_linux_amd64.tar.gz"
      sha256 "721d3b150f7cf1989a7c65d22c42dd757216045911dfef8c357307ffe683f761"
    end
  end

  def install
    bin.install "kaneo-cli"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/kaneo-cli version")
  end
end
