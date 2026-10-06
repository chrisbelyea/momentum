project('momentum-linux', 'vala', version: '0.1.0', meson_version: '>=0.59')

install_data('org.momentum.Linux.desktop', install_dir: join_paths(get_option('datadir'), 'applications'))

executable('momentum-linux', 'main.vala',
  dependencies: [
    dependency('gtk4', version: '>=4.6'),
    dependency('libsoup-3.0'),
    dependency('json-glib-1.0'),
    dependency('libsecret-1'),
  ],
  install: true,
)
