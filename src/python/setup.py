#!/usr/bin/env python3
# Copyright (C) 2019-2024, Lux Industries Inc. All rights reserved.
# See the file LICENSE for licensing terms.

from setuptools import setup, Extension
from Cython.Build import cythonize
import os
import sys
import platform

# Path to the C library
c_lib_path = os.path.join(os.path.dirname(__file__), '..', 'c')
include_dirs = [os.path.join(c_lib_path, 'include')]
library_dirs = [os.path.join(c_lib_path, 'lib')]

# Platform-specific settings
extra_link_args = []
if platform.system() == 'Darwin':
    # macOS specific settings
    extra_link_args = ['-Wl,-rpath,@loader_path/../c/lib']
elif platform.system() == 'Linux':
    # Linux specific settings
    extra_link_args = ['-Wl,-rpath,$ORIGIN/../c/lib']

extensions = [
    Extension(
        "lux_consensus",
        ["lux_consensus.pyx"],
        include_dirs=include_dirs,
        library_dirs=library_dirs,
        libraries=["luxconsensus"],
        extra_link_args=extra_link_args,
        language="c",
    )
]

setup(
    name="lux-consensus",
    version="0.1.0",
    description="Python bindings for Lux Consensus C library",
    author="Lux Industries Inc.",
    ext_modules=cythonize(extensions, language_level="3"),
    zip_safe=False,
    python_requires=">=3.7",
)