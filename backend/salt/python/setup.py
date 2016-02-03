#!/usr/bin/python
#
# Copyright 2015 Red Hat, Inc.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

from setuptools import setup, find_packages

setup(
    name="skyring",
    version="0.0.1",
    description='Python wrappers for SkyRing',
    license='Apache-2.0',
    author='Red Hat, Inc.',
    author_email='skyring@redhat.com',
    url='github.com/skyrings/skyring',
    packages = find_packages(exclude=['test']),
    test_suite='nose.collector',
    zip_safe=False,
)
