class Tmh < Formula
  desc "TUI hub for tmux: declarative sessions, drift sync, dotfile reload"
  homepage "https://github.com/mark1708/tmh"
  version "0.1.0"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/mark1708/tmh/releases/download/v#{version}/tmh_#{version}_darwin_arm64.tar.gz"
      sha256 "REPLACE_WITH_DARWIN_ARM64_SHA256"
    end
    if Hardware::CPU.intel?
      url "https://github.com/mark1708/tmh/releases/download/v#{version}/tmh_#{version}_darwin_amd64.tar.gz"
      sha256 "REPLACE_WITH_DARWIN_AMD64_SHA256"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/mark1708/tmh/releases/download/v#{version}/tmh_#{version}_linux_arm64.tar.gz"
      sha256 "REPLACE_WITH_LINUX_ARM64_SHA256"
    end
    if Hardware::CPU.intel?
      url "https://github.com/mark1708/tmh/releases/download/v#{version}/tmh_#{version}_linux_amd64.tar.gz"
      sha256 "REPLACE_WITH_LINUX_AMD64_SHA256"
    end
  end

  depends_on "tmux" => "3.2"

  def install
    bin.install "tmh"
    # goreleaser tarballs ship pre-generated man pages and completions
    # under docs/ — install them into the Homebrew-standard locations so
    # `man tmh` and tab-completion work out of the box.
    man1.install Dir["docs/man/*.1"] if Dir.exist?("docs/man")
    if Dir.exist?("docs/completions")
      bash_completion.install "docs/completions/bash/tmh"
      zsh_completion.install "docs/completions/zsh/tmh" => "_tmh"
      fish_completion.install "docs/completions/fish/tmh" => "tmh.fish"
    else
      # Fallback for source builds that don't include pre-generated docs.
      generate_completions_from_executable(bin/"tmh", "completion")
    end
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/tmh version")
  end
end
