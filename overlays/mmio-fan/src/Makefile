obj-m += mmio_fan.o

KERNEL_SRC ?= /lib/modules/$(shell uname -r)/build

all default: modules

modules modules_install clean:
	$(MAKE) -C $(KERNEL_SRC) M=$(CURDIR) $@

install: modules_install

.PHONY: all default modules modules_install clean install
