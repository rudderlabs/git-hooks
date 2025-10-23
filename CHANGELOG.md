# Changelog

## [1.1.3](https://github.com/rudderlabs/git-hooks/compare/v1.1.2...v1.1.3) (2025-10-23)


### Bug Fixes

* disable component in release tags to match existing tag format ([397b549](https://github.com/rudderlabs/git-hooks/commit/397b549d9714bd2c07079a0b1ee8835ab93544a1))
* test it ([1685a0e](https://github.com/rudderlabs/git-hooks/commit/1685a0ed12e0791726133c1f3c147df6115ccdb0))
* test release-please with github app token and chore commits ([aba015f](https://github.com/rudderlabs/git-hooks/commit/aba015f79efcadb92c663512a9a9a1e16df20def))
* update release-please workflow to use correct branch extraction ([c1fea2e](https://github.com/rudderlabs/git-hooks/commit/c1fea2e4f45d5045513b53df0fc210ba1706cd78))


### Miscellaneous

* add app id ([5f0ca94](https://github.com/rudderlabs/git-hooks/commit/5f0ca94127c8027628ec1783fb3495dc605cfcdf))
* add manifest file and update workflow to use config/manifest mode ([cd6ce52](https://github.com/rudderlabs/git-hooks/commit/cd6ce526e09dc4c48ba91ef1b7aa9fccfaecfdb3))
* add release-please configuration file ([2a8d236](https://github.com/rudderlabs/git-hooks/commit/2a8d236ef25fb4f3f73adc5ddcc652b83733df25))
* forcing release 0.1.0 ([d5cf1cb](https://github.com/rudderlabs/git-hooks/commit/d5cf1cb5c2691f085a15e821d965622c6c7f5ada))
* remove workflow_dispatch from release-please workflow ([f28144e](https://github.com/rudderlabs/git-hooks/commit/f28144ea51ed176b8203c182b0894e770ff3d155))
* rename GitHub App credentials variables ([b490b4a](https://github.com/rudderlabs/git-hooks/commit/b490b4a83e8938140a823266e1f47e6f18ab78ec))
* update release-please action to googleapis version ([bc0fdd4](https://github.com/rudderlabs/git-hooks/commit/bc0fdd453c94436b51c4000f6061db7bd5f7285f))
* use github app token for release please ([a047fce](https://github.com/rudderlabs/git-hooks/commit/a047fce0fbe4d1bbc7333768ff5fcb206c8c41e0))

## [1.1.2](https://github.com/rudderlabs/git-hooks/compare/v1.1.1...v1.1.2) (2025-10-22)


### Bug Fixes

* wrong path in module ([#27](https://github.com/rudderlabs/git-hooks/issues/27)) ([c16b938](https://github.com/rudderlabs/git-hooks/commit/c16b9388f5ccf9a6d2bfa893ee6c8d85870cc717))

## [1.1.1](https://github.com/rudderlabs/git-hooks/compare/v1.1.0...v1.1.1) (2025-10-21)


### Bug Fixes

* broken release ([#24](https://github.com/rudderlabs/git-hooks/issues/24)) ([ed3d1e3](https://github.com/rudderlabs/git-hooks/commit/ed3d1e310f95bc2de4b42353bf9baaa5b7e78e9c))

## [1.1.0](https://github.com/rudderlabs/git-hooks/compare/v1.0.2...v1.1.0) (2025-10-21)


### Features

* add local repo cleanup hooks ([#21](https://github.com/rudderlabs/git-hooks/issues/21)) ([637d5b0](https://github.com/rudderlabs/git-hooks/commit/637d5b0ff2abac67a4de986dc4f49f5cc8ae43ad))
* tag commit message with gitleaks version using commit-msg hook ([#17](https://github.com/rudderlabs/git-hooks/issues/17)) ([5ea3ead](https://github.com/rudderlabs/git-hooks/commit/5ea3eaddaf28a1aee2182cd185dab86b0858c09b))

## [1.0.2](https://github.com/rudderlabs/git-hooks/compare/v1.0.1...v1.0.2) (2025-09-23)


### Bug Fixes

* husky hooks legacy and modern dirs ([b30c802](https://github.com/rudderlabs/git-hooks/commit/b30c8023038a1aa65440bb3ee0ceeeb2ca1e3d12))

## 1.0.0 (2025-01-08)


### Features

* husky support ([641dd97](https://github.com/rudderlabs/git-hooks/commit/641dd9742a4467a0f44c62bddfd8800676840e69))
* implement hierarchical Git hook configuration and execution ([253fa99](https://github.com/rudderlabs/git-hooks/commit/253fa99f8e2c1f7f5228e5009daec180ee4f0da5))
* improve gitleaks hook ([cfe52fc](https://github.com/rudderlabs/git-hooks/commit/cfe52fcf810e41bd53d78ac7f26b4b5ab7afd055))
* support impode command ([82ef820](https://github.com/rudderlabs/git-hooks/commit/82ef82041c69611ec926f042c4bf2135297f0def))


### Bug Fixes

* husky hooks execution ([69b3a1b](https://github.com/rudderlabs/git-hooks/commit/69b3a1bd93da2b37a0555899a566fbc118ca3cf3))
* improve hook script robustness ([cae3750](https://github.com/rudderlabs/git-hooks/commit/cae37504654ecea4616fc66ecf1ef931bbcc167a))
* README.md ([ea25d1d](https://github.com/rudderlabs/git-hooks/commit/ea25d1da09c67fd3947c5be1e8ba26898490250a))
* run goreleaser only against tags ([6982032](https://github.com/rudderlabs/git-hooks/commit/6982032e50f79abccba8070626e580ca535699ba))
* use full path for calling git-hooks ([64b580f](https://github.com/rudderlabs/git-hooks/commit/64b580fe88dc04e744fcdcf03ded7d4f29604cba))
* use full path for gitleaks ([70a27c6](https://github.com/rudderlabs/git-hooks/commit/70a27c623c68ad0ceee129d5af1e5625fbe4266d))

## [0.2.0](https://github.com/rudderlabs/git-hooks/compare/v0.1.5...v0.2.0) (2024-12-12)


### Features

* husky support ([641dd97](https://github.com/rudderlabs/git-hooks/commit/641dd9742a4467a0f44c62bddfd8800676840e69))
* implement hierarchical Git hook configuration and execution ([253fa99](https://github.com/rudderlabs/git-hooks/commit/253fa99f8e2c1f7f5228e5009daec180ee4f0da5))
* improve gitleaks hook ([cfe52fc](https://github.com/rudderlabs/git-hooks/commit/cfe52fcf810e41bd53d78ac7f26b4b5ab7afd055))
* support impode command ([82ef820](https://github.com/rudderlabs/git-hooks/commit/82ef82041c69611ec926f042c4bf2135297f0def))


### Bug Fixes

* improve hook script robustness ([cae3750](https://github.com/rudderlabs/git-hooks/commit/cae37504654ecea4616fc66ecf1ef931bbcc167a))
* README.md ([ea25d1d](https://github.com/rudderlabs/git-hooks/commit/ea25d1da09c67fd3947c5be1e8ba26898490250a))
* run goreleaser only against tags ([6982032](https://github.com/rudderlabs/git-hooks/commit/6982032e50f79abccba8070626e580ca535699ba))
* use full path for calling git-hooks ([64b580f](https://github.com/rudderlabs/git-hooks/commit/64b580fe88dc04e744fcdcf03ded7d4f29604cba))
* use full path for gitleaks ([70a27c6](https://github.com/rudderlabs/git-hooks/commit/70a27c623c68ad0ceee129d5af1e5625fbe4266d))


### Miscellaneous

* add go releaser ([9eb798e](https://github.com/rudderlabs/git-hooks/commit/9eb798e0fe83e270186621c06c61cc1a8e7f5388))
* add README.md ([e53c93b](https://github.com/rudderlabs/git-hooks/commit/e53c93bac0bc791d6a79b3571e4acaea8cac69c6))
* add release please ([78801e0](https://github.com/rudderlabs/git-hooks/commit/78801e08017408717a71f0f4ee711231826c3e39))
* add release please ([103e946](https://github.com/rudderlabs/git-hooks/commit/103e946e7993826a0f0f8e7b4d359e767d937758))
* dependencies ([a669ea3](https://github.com/rudderlabs/git-hooks/commit/a669ea34a9b36d900627acead42fcf69b99c393a))
* empty ([0f07fdc](https://github.com/rudderlabs/git-hooks/commit/0f07fdcacccbf4af8def418c9cb575419bd10882))
* fix goreleaser yml ([8f8884b](https://github.com/rudderlabs/git-hooks/commit/8f8884b039667a11cea4a24fe63047821e082fb2))
* fix token name ([af85e28](https://github.com/rudderlabs/git-hooks/commit/af85e28f9b8226948f6301d23ae4982faadfb791))
* goreleaser don't skip upload ([b86c92f](https://github.com/rudderlabs/git-hooks/commit/b86c92f6f21b23aae185fd10e2181b7da194924d))
* **goreleaser:** cleanup ([db67de9](https://github.com/rudderlabs/git-hooks/commit/db67de9b5bf2b1c6735f8823c311307ca987c601))
* **goreleaser:** use homebrew-tap repo ([3927761](https://github.com/rudderlabs/git-hooks/commit/3927761e686e2a81d57c7084dad1f50b1ec71477))
* **goreleaser:** use token for external repo ([69ab0cd](https://github.com/rudderlabs/git-hooks/commit/69ab0cd4a93eb6ffbcc26763dd2c4fec8e810408))
* **goreleaser:** use token for external repo ([326c7e2](https://github.com/rudderlabs/git-hooks/commit/326c7e23e626c7b98564d912b868e38f1e550d8f))
* print path if git-hooks is not found ([4ea57b0](https://github.com/rudderlabs/git-hooks/commit/4ea57b018946b1fbc43773b07050728fa7e26653))
* release 0.1.0 ([#1](https://github.com/rudderlabs/git-hooks/issues/1)) ([5dfa576](https://github.com/rudderlabs/git-hooks/commit/5dfa5768ed49b0d2e580d1e68a2920407bd2e7ee))
* release 0.1.1 ([#2](https://github.com/rudderlabs/git-hooks/issues/2)) ([8461d83](https://github.com/rudderlabs/git-hooks/commit/8461d83f29e7ad96b2bbc0a840f4ba0b6d90a2fa))
* release 0.1.2 ([#3](https://github.com/rudderlabs/git-hooks/issues/3)) ([60d5f55](https://github.com/rudderlabs/git-hooks/commit/60d5f55e073cae1744bea8503820e0a224d4dfcb))
* release 0.1.4 ([#4](https://github.com/rudderlabs/git-hooks/issues/4)) ([46c7c6a](https://github.com/rudderlabs/git-hooks/commit/46c7c6af64c07d2bc84017c63e28fefa2a711e4c))
* release 0.1.5 ([#6](https://github.com/rudderlabs/git-hooks/issues/6)) ([db0d841](https://github.com/rudderlabs/git-hooks/commit/db0d841092af6944ec12cf00b645ac65bfee3b27))
* remove git from brew deps ([730e3a4](https://github.com/rudderlabs/git-hooks/commit/730e3a452fd281098fe9da4ba53f0d78eb27d263))

## [0.1.5](https://github.com/lvrach/git-hooks/compare/v0.1.4...v0.1.5) (2024-10-17)


### Miscellaneous

* remove git from brew deps ([730e3a4](https://github.com/lvrach/git-hooks/commit/730e3a452fd281098fe9da4ba53f0d78eb27d263))

## [0.1.4](https://github.com/lvrach/git-hooks/compare/v0.1.3...v0.1.4) (2024-10-16)


### Miscellaneous

* fix token name ([af85e28](https://github.com/lvrach/git-hooks/commit/af85e28f9b8226948f6301d23ae4982faadfb791))

## [0.1.2](https://github.com/lvrach/git-hooks/compare/v0.1.1...v0.1.2) (2024-10-16)


### Miscellaneous

* empty ([0f07fdc](https://github.com/lvrach/git-hooks/commit/0f07fdcacccbf4af8def418c9cb575419bd10882))

## [0.1.1](https://github.com/lvrach/git-hooks/compare/v0.1.0...v0.1.1) (2024-10-16)


### Miscellaneous

* add release please ([78801e0](https://github.com/lvrach/git-hooks/commit/78801e08017408717a71f0f4ee711231826c3e39))

## [0.1.0](https://github.com/lvrach/git-hooks/compare/v0.0.1...v0.1.0) (2024-10-16)


### Features

* husky support ([641dd97](https://github.com/lvrach/git-hooks/commit/641dd9742a4467a0f44c62bddfd8800676840e69))


### Miscellaneous

* add release please ([103e946](https://github.com/lvrach/git-hooks/commit/103e946e7993826a0f0f8e7b4d359e767d937758))
* dependencies ([a669ea3](https://github.com/lvrach/git-hooks/commit/a669ea34a9b36d900627acead42fcf69b99c393a))
* fix goreleaser yml ([8f8884b](https://github.com/lvrach/git-hooks/commit/8f8884b039667a11cea4a24fe63047821e082fb2))
* goreleaser don't skip upload ([b86c92f](https://github.com/lvrach/git-hooks/commit/b86c92f6f21b23aae185fd10e2181b7da194924d))
* **goreleaser:** cleanup ([db67de9](https://github.com/lvrach/git-hooks/commit/db67de9b5bf2b1c6735f8823c311307ca987c601))
* **goreleaser:** use homebrew-tap repo ([3927761](https://github.com/lvrach/git-hooks/commit/3927761e686e2a81d57c7084dad1f50b1ec71477))
* **goreleaser:** use token for external repo ([69ab0cd](https://github.com/lvrach/git-hooks/commit/69ab0cd4a93eb6ffbcc26763dd2c4fec8e810408))
* **goreleaser:** use token for external repo ([326c7e2](https://github.com/lvrach/git-hooks/commit/326c7e23e626c7b98564d912b868e38f1e550d8f))
