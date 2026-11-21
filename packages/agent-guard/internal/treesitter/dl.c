#include <dlfcn.h>
#include "dl.h"

typedef const void *(*language_fn)(void);

const void *grammar_load(const char *path, const char *symbol, const char **err) {
	void *handle = dlopen(path, RTLD_NOW | RTLD_LOCAL);
	if (!handle) {
		*err = dlerror();
		return NULL;
	}
	void *fn = dlsym(handle, symbol);
	if (!fn) {
		*err = dlerror();
		dlclose(handle);
		return NULL;
	}
	return ((language_fn)fn)();
}
