{
  lib,
  stdenv,
  fetchurl,
  vulkan-headers,
}:

stdenv.mkDerivation (finalAttrs: {
  pname = "nms-vk-fix";
  version = "0-unstable-2026-09-30";

  src = fetchurl {
    url = "https://github.com/user-attachments/files/32835125/nms_fix.c.txt";
    hash = "sha256-sVZOi2l1EzY8nAay1oJtdC7Mq0HNPpo6qZwUspzGifA=";
  };

  dontUnpack = true;

  buildInputs = [ vulkan-headers ];

  buildPhase = ''
    runHook preBuild
    $CC -O2 -fPIC -shared -fvisibility=hidden -Wall -o libnms_fix.so -x c $src -lpthread
    runHook postBuild
  '';

  layerManifest = builtins.toJSON {
    file_format_version = "1.0.0";
    layer = {
      name = "VK_LAYER_NMS_fix";
      type = "GLOBAL";
      library_path = "@out@/lib/libnms_fix.so";
      api_version = "1.3.0";
      implementation_version = "1";
      description = "Shadow binary semaphores waited by two queues in Streamline DLSS-G";
      functions = {
        vkGetInstanceProcAddr = "nms_fix_GetInstanceProcAddr";
        vkGetDeviceProcAddr = "nms_fix_GetDeviceProcAddr";
      };
    };
  };
  passAsFile = [ "layerManifest" ];

  installPhase = ''
    runHook preInstall
    install -Dm755 libnms_fix.so $out/lib/libnms_fix.so
    install -Dm644 $layerManifestPath $out/share/vulkan/explicit_layer.d/VkLayer_NMS_fix.json
    substituteInPlace $out/share/vulkan/explicit_layer.d/VkLayer_NMS_fix.json --replace-fail "@out@" "$out"
    runHook postInstall
  '';

  meta = {
    description = "Vulkan layer working around the Streamline DLSS-G binary semaphore deadlock in No Man's Sky under Proton";
    homepage = "https://github.com/ValveSoftware/Proton/issues/438#issuecomment-5902067647";
    license = lib.licenses.unfree;
    platforms = [ "x86_64-linux" ];
  };
})
