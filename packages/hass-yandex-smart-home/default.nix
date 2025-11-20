{
  lib,
  buildHomeAssistantComponent,
  fetchFromGitHub,
  home-assistant,
}:
buildHomeAssistantComponent rec {
  owner = "dext0r";
  domain = "yandex_smart_home";
  version = "1.0.2";

  src = fetchFromGitHub {
    owner = "dext0r";
    repo = "yandex_smart_home";
    tag = "v${version}";
    hash = "sha256-ZlVrAK7NXvRiVN1mGHovSy9Szj+uqH6cETQKPRBFmR8=";
  };

  dependencies = with home-assistant.python.pkgs; [
    pydantic
  ];

  meta = {
    changelog = "https://github.com/dext0r/yandex_smart_home/releases/tag/v${src.tag}";
    description = "Управление устройствами из Home Assistant через Алису (Умный дом Яндекса) или Марусю";
    homepage = "https://github.com/dext0r/yandex_smart_home";
    license = lib.licenses.mit;
    maintainers = with lib.maintainers; [ elxreno ];
  };
}
