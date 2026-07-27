# Maintainer: Fran <francorbacho@proton.me>

pkgname=piensa
pkgver=__VERSION__
pkgrel=1
pkgdesc="PiensaSolutions VPS manager CLI"
arch=('x86_64' 'aarch64')
url="https://github.com/francorbacho/terraform-provider-piensasolutions"
license=('MIT')
depends=('glibc')
makedepends=('go')
source=("${pkgname}-${pkgver}.tar.gz::https://github.com/francorbacho/terraform-provider-piensasolutions/archive/v${pkgver}.tar.gz")
b2sums=('SKIP')

build() {
  cd "${srcdir}/${pkgname}"
  go build -ldflags "-X main.version=${pkgver}" -o piensa ./cmd/piensa/
}

package() {
  cd "${srcdir}/${pkgname}"
  install -Dm755 piensa "${pkgdir}/usr/bin/piensa"
}
