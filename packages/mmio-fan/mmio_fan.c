// SPDX-License-Identifier: GPL-2.0-only
/*
 * mmio_fan - MMIO GPIO fan thermal cooling device
 *
 * Exposes a GPIO-controlled fan as a thermal cooling device for thermald.
 * The fan is toggled via a single bit in a memory-mapped GPIO pad register,
 * bypassing the pinctrl driver (which refuses writes on locked pads).
 *
 * Written for MOREFINE M6S (Intel Jasper Lake N5105) where the fan is
 * connected to GPP_C7 at MMIO 0xFD6A0670, bit 0 = TX state (on/off).
 */

#include <linux/module.h>
#include <linux/thermal.h>
#include <linux/io.h>

static unsigned long pad_addr = 0xFD6A0670;
module_param(pad_addr, ulong, 0444);
MODULE_PARM_DESC(pad_addr, "GPIO pad DW0 MMIO address (default: 0xFD6A0670)");

static unsigned int fan_bit = 0;
module_param(fan_bit, uint, 0444);
MODULE_PARM_DESC(fan_bit, "Bit index in DW0 for fan TX state (default: 0)");

static void __iomem *pad_base;
static struct thermal_cooling_device *cdev;

static int fan_get_max_state(struct thermal_cooling_device *cdev,
			     unsigned long *state)
{
	*state = 1;
	return 0;
}

static int fan_get_cur_state(struct thermal_cooling_device *cdev,
			     unsigned long *state)
{
	u32 val = readl(pad_base);
	*state = (val >> fan_bit) & 1;
	return 0;
}

static int fan_set_cur_state(struct thermal_cooling_device *cdev,
			     unsigned long state)
{
	u32 val = readl(pad_base);

	if (state)
		val |= BIT(fan_bit);
	else
		val &= ~BIT(fan_bit);

	writel(val, pad_base);
	return 0;
}

static const struct thermal_cooling_device_ops fan_cooling_ops = {
	.get_max_state = fan_get_max_state,
	.get_cur_state = fan_get_cur_state,
	.set_cur_state = fan_set_cur_state,
};

static int __init mmio_fan_init(void)
{
	pad_base = ioremap(pad_addr, 4);
	if (!pad_base) {
		pr_err("mmio_fan: failed to ioremap 0x%lx\n", pad_addr);
		return -ENOMEM;
	}

	cdev = thermal_cooling_device_register("mmio-fan", NULL,
					       &fan_cooling_ops);
	if (IS_ERR(cdev)) {
		iounmap(pad_base);
		pr_err("mmio_fan: failed to register cooling device\n");
		return PTR_ERR(cdev);
	}

	pr_info("mmio_fan: registered cooling device (pad=0x%lx bit=%u)\n",
		pad_addr, fan_bit);
	return 0;
}

static void __exit mmio_fan_exit(void)
{
	thermal_cooling_device_unregister(cdev);
	iounmap(pad_base);
	pr_info("mmio_fan: unregistered\n");
}

module_init(mmio_fan_init);
module_exit(mmio_fan_exit);

MODULE_LICENSE("GPL");
MODULE_AUTHOR("ElXreno");
MODULE_DESCRIPTION("MMIO GPIO fan thermal cooling device");
