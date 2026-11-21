{
  lib,
  rustPlatform,
  pkg-config,
  oniguruma,
  onnxruntime,
  mclSrc,
}:

rustPlatform.buildRustPackage {
  pname = "manticore-knn-embeddings";
  version = "1.1.1";

  src = "${mclSrc}/embeddings";

  cargoHash = "sha256-8rDi/CrHvt3a5f8a3IdNapTWhFgNEQ++OfF7oaeOoM4=";

  nativeBuildInputs = [ pkg-config ];

  buildInputs = [
    oniguruma
    onnxruntime
  ];

  env = {
    ORT_LIB_LOCATION = "${lib.getLib onnxruntime}/lib";
    ORT_PREFER_DYNAMIC_LINK = "1";
    RUSTONIG_SYSTEM_LIBONIG = "1";
    GIT_COMMIT_ID = "nixpkgs";
    GIT_TIMESTAMP_ID = "00000000";
  };

  doCheck = false;

  postInstall = ''
    install -Dm755 $out/lib/libmanticore_knn_embeddings.so $out/share/manticore/modules/lib_manticore_knn_embeddings.so
    rm -r $out/lib
  '';

  meta = {
    description = "Text embeddings library for Manticore KNN auto-embeddings";
    homepage = "https://github.com/manticoresoftware/columnar/tree/master/embeddings";
    license = lib.licenses.asl20;
    platforms = lib.platforms.linux;
  };
}
