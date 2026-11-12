# TODO

## Release automation

- [ ] Create a fine-grained GitHub token for `mc-akash/homebrew-tap` only,
      with Contents: Read and write.
- [ ] Store it: `gh secret set HOMEBREW_TAP_TOKEN --repo mc-akash/lazypost`
- [ ] Push the next tag (`git tag v0.1.1 && git push origin v0.1.1`) and
      check the `release` workflow run.
- [ ] After that run: delete `Formula/lazypost.rb` from `homebrew-tap`
      (GoReleaser writes `Casks/lazypost.rb`; both have the same name).
- [ ] Test `brew install mc-akash/tap/lazypost` on a Mac.

## AUR (registration is closed for now)

- [ ] Create an account on aur.archlinux.org.
- [ ] Add `~/.ssh/my_id_ed25519.pub` to the account.
- [ ] `cd ~/code_base/github.com/mc-akash/aur-lazypost && git push -u origin master`
- [ ] Optional: let GoReleaser publish to the AUR (`aur_sources` in
      `.goreleaser.yaml`, plus an `AUR_KEY` secret).

## Later

- [ ] Windows builds.
- [ ] Sign and notarize the macOS binary, then drop the quarantine hook.
