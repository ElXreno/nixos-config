{ namespace, ... }:
{
  ${namespace} = {
    roles.tablet.touchscreen = "/dev/input/by-id/usb-Atmel_Atmel_maXTouch_Digitizer-event-if00";
  };

  programs.niri.settings.binds."XF86Launch1".action.toggle-overview = [ ];
}
