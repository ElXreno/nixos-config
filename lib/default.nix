{ inputs, ... }:
{
  # Stable integer bucket that flips every `days` of wall-clock flake
  # activity. Use as a `clan.core.vars.generators.<name>.validation` value
  # to rotate secrets on a coarse cadence.
  mkRotationBucket = days: toString ((inputs.self.lastModified or 0) / (days * 24 * 3600));
}
