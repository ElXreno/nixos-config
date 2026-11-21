{
  config,
  lib,
  s4,
  mkClanSecret,
  ...
}:
let
  secret = name: config.data.external.${name} "result.secret";

  bucketName = "kopia-backups";

  accountId = secret "s4-account-id";
  bucket = config.resource.aws_s3_bucket.kopia "id";
  zerofsBucket = config.resource.aws_s3_bucket.zerofs "id";

  userArn = machine: "arn:aws:iam::${accountId}:user/${machine.user}";

  bucketPolicy =
    bucket: objects: machines:
    builtins.toJSON {
      Version = "2012-10-17";
      Statement = lib.concatMap (machine: [
        {
          Sid = "${machine.name}-objects";
          Effect = "Allow";
          Principal.AWS = userArn machine;
          Action = "s3:*";
          Resource = objects machine;
        }
        {
          Sid = "${machine.name}-bucket";
          Effect = "Allow";
          Principal.AWS = userArn machine;
          Action = [
            "s3:ListBucket"
            "s3:GetBucketLocation"
          ];
          Resource = "arn:aws:s3:::${bucket}";
        }
      ]) machines;
    };
in
{
  terraform.required_providers.aws.source = "hashicorp/aws";

  data.external = {
    s4-access-key = mkClanSecret "s4-access-key";
    s4-secret-key = mkClanSecret "s4-secret-key";
    s4-account-id = mkClanSecret "s4-account-id";
    s4-endpoint = mkClanSecret "s4-endpoint";
    s4-region = mkClanSecret "s4-region";
  };

  provider.aws = {
    access_key = secret "s4-access-key";
    secret_key = secret "s4-secret-key";
    region = secret "s4-region";
    skip_credentials_validation = true;
    skip_requesting_account_id = true;
    skip_region_validation = true;
    endpoints.s3 = "https://${secret "s4-endpoint"}";
  };

  resource.aws_s3_bucket = {
    kopia = {
      bucket = bucketName;
      lifecycle = [ { prevent_destroy = true; } ];
    };
    zerofs.bucket = "zerofs";
  };

  resource.aws_s3_bucket_policy = {
    kopia = {
      inherit bucket;
      policy = bucketPolicy bucket (
        machine: map (prefix: "arn:aws:s3:::${bucket}/${prefix}/*") machine.prefixes
      ) s4.machines;
    };
    zerofs = {
      bucket = zerofsBucket;
      policy = bucketPolicy zerofsBucket (
        machine: "arn:aws:s3:::${zerofsBucket}/${machine.prefix}*"
      ) s4.zerofs;
    };
  };
}
