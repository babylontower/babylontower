package ui

import (
	"fmt"
	"image"
	"image/color"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	babylonapp "babylontower/pkg/app"
	"babylontower/pkg/groups"
)

// ── Group Data Refresh ──────────────────────────────────────────────────────

func (a *App) refreshGroups() {
	if a.uiGroupMgr == nil {
		return
	}
	go func() {
		groupInfos, err := a.uiGroupMgr.ListGroups()
		if err != nil {
			logger.Warnw("failed to list groups", "error", err)
			return
		}
		select {
		case a.uiEvents <- uiEvent{Type: evtGroupsRefreshed, Groups: groupInfos}:
		default:
		}
		a.window.Invalidate()
	}()
}

func (a *App) selectGroup(groupID string) {
	a.selectedGroupID = groupID
	// Clear contact selection when selecting a group
	a.selectedKey = ""
}

func (a *App) selectedGroup() *babylonapp.GroupInfo {
	for _, g := range a.groupList {
		if g.GroupID == a.selectedGroupID {
			return g
		}
	}
	return nil
}

// ── Sidebar Tab Navigation ──────────────────────────────────────────────────

func (a *App) layoutSidebarTabs(gtx layout.Context) layout.Dimensions {
	chatsActive := a.sidebarTab == "chats"

	return layout.Inset{Left: unit.Dp(12), Right: unit.Dp(12), Top: unit.Dp(4), Bottom: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Spacing: layout.SpaceEvenly}.Layout(gtx,
			// Chats tab
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return material.Clickable(gtx, &a.ui.chatsTabBtn, func(gtx layout.Context) layout.Dimensions {
					return a.layoutTabButton(gtx, "CHATS", chatsActive)
				})
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
			// Groups tab
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return material.Clickable(gtx, &a.ui.groupsTabBtn, func(gtx layout.Context) layout.Dimensions {
					return a.layoutTabButton(gtx, "GROUPS", !chatsActive)
				})
			}),
		)
	})
}

func (a *App) layoutTabButton(gtx layout.Context, label string, active bool) layout.Dimensions {
	textColor := color.NRGBA(a.ui.theme.TextSecondary)
	if active {
		textColor = color.NRGBA(a.ui.theme.Accent)
	}

	return layout.Stack{Alignment: layout.Center}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			sz := gtx.Constraints.Min
			h := gtx.Dp(28)
			if sz.Y < h {
				sz.Y = h
			}
			if active {
				// Underline indicator
				lineH := gtx.Dp(2)
				lineY := sz.Y - lineH
				rect := clip.Rect{Min: image.Pt(0, lineY), Max: image.Pt(sz.X, sz.Y)}
				paint.FillShape(gtx.Ops, color.NRGBA(a.ui.theme.Accent), rect.Op())
			}
			return layout.Dimensions{Size: image.Pt(sz.X, h)}
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			l := material.Caption(a.theme, label)
			l.Color = textColor
			l.Font.Weight = font.Bold
			l.TextSize = unit.Sp(11)
			l.Alignment = text.Middle
			return l.Layout(gtx)
		}),
	)
}

// ── Group List (sidebar) ────────────────────────────────────────────────────

func (a *App) layoutGroupsTab(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		// Header with "+" create group button
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: unit.Dp(16), Right: unit.Dp(8), Top: unit.Dp(8), Bottom: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						l := material.Caption(a.theme, "Your Groups")
						l.Color = color.NRGBA(a.ui.theme.TextSecondary)
						l.TextSize = unit.Sp(10)
						return l.Layout(gtx)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return material.Clickable(gtx, &a.ui.createGroupBtn, func(gtx layout.Context) layout.Dimensions {
							return layout.Inset{Left: unit.Dp(8), Right: unit.Dp(8), Top: unit.Dp(4), Bottom: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								l := material.Body1(a.theme, "+")
								l.Color = color.NRGBA(a.ui.theme.Accent)
								l.Font.Weight = font.Bold
								l.TextSize = unit.Sp(18)
								return l.Layout(gtx)
							})
						})
					}),
				)
			})
		}),
		// Group list
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if len(a.groupList) == 0 {
				return a.layoutEmptyGroups(gtx)
			}
			return material.List(a.theme, &a.ui.groupList).Layout(gtx, len(a.groupList), func(gtx layout.Context, index int) layout.Dimensions {
				return a.layoutGroupItem(gtx, index)
			})
		}),
	)
}

func (a *App) layoutEmptyGroups(gtx layout.Context) layout.Dimensions {
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				l := material.Body2(a.theme, "No groups yet")
				l.Color = color.NRGBA(a.ui.theme.TextSecondary)
				l.Alignment = text.Middle
				return l.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				l := material.Caption(a.theme, "Press + to create a group")
				l.Color = color.NRGBA(a.ui.theme.Accent)
				l.Alignment = text.Middle
				return l.Layout(gtx)
			}),
		)
	})
}

func (a *App) layoutGroupItem(gtx layout.Context, index int) layout.Dimensions {
	if index >= len(a.groupList) {
		return layout.Dimensions{}
	}
	group := a.groupList[index]

	for len(a.ui.groupButtons) <= index {
		a.ui.groupButtons = append(a.ui.groupButtons, widget.Clickable{})
	}

	isSelected := group.GroupID == a.selectedGroupID

	return layout.Inset{Left: unit.Dp(4), Right: unit.Dp(4), Top: unit.Dp(1), Bottom: unit.Dp(1)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return material.Clickable(gtx, &a.ui.groupButtons[index], func(gtx layout.Context) layout.Dimensions {
			if isSelected {
				sz := gtx.Constraints.Max
				rr := gtx.Dp(4)
				highlightColor := a.ui.theme.Accent
				highlightColor.A = 30
				rect := clip.RRect{Rect: image.Rect(0, 0, sz.X, gtx.Dp(52)), NE: rr, NW: rr, SE: rr, SW: rr}
				paint.FillShape(gtx.Ops, highlightColor, rect.Op(gtx.Ops))
			}

			return layout.Inset{Left: unit.Dp(12), Right: unit.Dp(12), Top: unit.Dp(8), Bottom: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					// Group icon (different from contact dot)
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return a.layoutGroupIcon(gtx, group.Type)
					}),
					// Name and info
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Left: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									nameColor := color.NRGBA(a.ui.theme.TextPrimary)
									if isSelected {
										nameColor = color.NRGBA(a.ui.theme.Accent)
									}
									l := material.Body2(a.theme, group.Name)
									l.Color = nameColor
									l.Font.Weight = font.Medium
									return l.Layout(gtx)
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									info := fmt.Sprintf("%s  %d members", group.TypeLabel, group.MemberCount)
									l := material.Caption(a.theme, info)
									l.Color = color.NRGBA(a.ui.theme.TextSecondary)
									l.TextSize = unit.Sp(10)
									return l.Layout(gtx)
								}),
							)
						})
					}),
					// Role badge
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if !group.IsOwner && !group.IsAdmin {
							return layout.Dimensions{}
						}
						label := "Admin"
						if group.IsOwner {
							label = "Owner"
						}
						return a.layoutRoleBadge(gtx, label)
					}),
				)
			})
		})
	})
}

func (a *App) layoutGroupIcon(gtx layout.Context, groupType groups.GroupType) layout.Dimensions {
	radius := gtx.Dp(12)
	size := image.Point{X: radius * 2, Y: radius * 2}

	iconColor := a.ui.theme.Primary
	iconColor.A = 160

	// Draw circle background
	defer clip.Ellipse{Max: size}.Push(gtx.Ops).Pop()
	paint.FillShape(gtx.Ops, iconColor, clip.Ellipse{Max: size}.Op(gtx.Ops))

	// Icon character based on type
	icon := "G" // private group
	switch groupType {
	case groups.PublicGroup:
		icon = "P"
	case groups.PrivateChannel:
		icon = "C"
	case groups.PublicChannel:
		icon = "B" // broadcast
	}

	return layout.Stack{Alignment: layout.Center}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			return layout.Dimensions{Size: size}
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			l := material.Caption(a.theme, icon)
			l.Color = color.NRGBA(a.ui.theme.TextPrimary)
			l.Font.Weight = font.Bold
			l.TextSize = unit.Sp(11)
			return l.Layout(gtx)
		}),
	)
}

func (a *App) layoutRoleBadge(gtx layout.Context, role string) layout.Dimensions {
	return layout.Stack{Alignment: layout.Center}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			sz := gtx.Constraints.Min
			minW := gtx.Dp(36)
			if sz.X < minW {
				sz.X = minW
			}
			rr := sz.Y / 2
			badgeColor := a.ui.theme.Accent
			badgeColor.A = 40
			rect := clip.RRect{Rect: image.Rect(0, 0, sz.X, sz.Y), NE: rr, NW: rr, SE: rr, SW: rr}
			paint.FillShape(gtx.Ops, badgeColor, rect.Op(gtx.Ops))
			return layout.Dimensions{Size: sz}
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: unit.Dp(6), Right: unit.Dp(6), Top: unit.Dp(2), Bottom: unit.Dp(2)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				l := material.Caption(a.theme, role)
				l.Color = color.NRGBA(a.ui.theme.Accent)
				l.TextSize = unit.Sp(8)
				l.Font.Weight = font.Bold
				return l.Layout(gtx)
			})
		}),
	)
}

// ── Group Chat View (replaces chat column when a group is selected) ─────────

func (a *App) layoutGroupChatColumn(gtx layout.Context) layout.Dimensions {
	group := a.selectedGroup()
	if group == nil {
		return a.layoutChatPlaceholder(gtx, "Select a group")
	}

	surfaceColor := a.ui.theme.Surface
	surfaceColor.A = 180
	paint.FillShape(gtx.Ops, surfaceColor, clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, gtx.Constraints.Max.Y)}.Op())

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		// Group header (clickable for detail)
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return a.layoutGroupChatHeader(gtx, group)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layoutHorizontalDivider(gtx, a.goldDividerColor())
		}),
		// Group info placeholder (no group messaging wired yet)
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.UniformInset(unit.Dp(16)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return a.layoutGroupInfoCard(gtx, group)
			})
		}),
	)
}

func (a *App) layoutGroupChatHeader(gtx layout.Context, group *babylonapp.GroupInfo) layout.Dimensions {
	return layout.Inset{Left: unit.Dp(20), Right: unit.Dp(20), Top: unit.Dp(14), Bottom: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return material.Clickable(gtx, &a.ui.chatHeaderBtn, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				// Group icon
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return a.layoutGroupIcon(gtx, group.Type)
					})
				}),
				// Group name
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					l := material.H6(a.theme, group.Name)
					l.Color = color.NRGBA(a.ui.theme.Accent)
					l.Font.Weight = font.Bold
					return l.Layout(gtx)
				}),
				// Member count
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Left: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						l := material.Caption(a.theme, fmt.Sprintf("%d members", group.MemberCount))
						l.Color = color.NRGBA(a.ui.theme.TextSecondary)
						l.TextSize = unit.Sp(10)
						return l.Layout(gtx)
					})
				}),
				// Arrow for detail
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Left: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						l := material.Caption(a.theme, "\u25B8") // ▸
						l.Color = color.NRGBA(a.ui.theme.TextSecondary)
						return l.Layout(gtx)
					})
				}),
			)
		})
	})
}

func (a *App) layoutGroupInfoCard(gtx layout.Context, group *babylonapp.GroupInfo) layout.Dimensions {
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		maxW := gtx.Dp(400)
		if gtx.Constraints.Max.X < maxW {
			maxW = gtx.Constraints.Max.X
		}
		gtx.Constraints.Max.X = maxW

		return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
			// Group name
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				l := material.H5(a.theme, group.Name)
				l.Color = color.NRGBA(a.ui.theme.Accent)
				l.Font.Weight = font.Bold
				l.Alignment = text.Middle
				return l.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
			// Description
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if group.Description == "" {
					return layout.Dimensions{}
				}
				l := material.Body2(a.theme, group.Description)
				l.Color = color.NRGBA(a.ui.theme.TextSecondary)
				l.Alignment = text.Middle
				return l.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return a.drawDecorativeLine(gtx, 120)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
			// Type + member count
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				info := fmt.Sprintf("%s  |  %d members  |  Epoch %d", group.TypeLabel, group.MemberCount, group.Epoch)
				l := material.Caption(a.theme, info)
				l.Color = color.NRGBA(a.ui.theme.TextSecondary)
				l.Alignment = text.Middle
				return l.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
			// Member list
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				l := material.Caption(a.theme, "MEMBERS")
				l.Color = color.NRGBA(a.ui.theme.Accent)
				l.Font.Weight = font.Bold
				l.TextSize = unit.Sp(10)
				return l.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return a.layoutGroupMembers(gtx, group)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
			// Action buttons
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return a.layoutGroupActions(gtx, group)
			}),
		)
	})
}

func (a *App) layoutGroupMembers(gtx layout.Context, group *babylonapp.GroupInfo) layout.Dimensions {
	if len(group.Members) == 0 {
		l := material.Caption(a.theme, "No members")
		l.Color = color.NRGBA(a.ui.theme.TextSecondary)
		return l.Layout(gtx)
	}

	// Ensure enough remove buttons
	for len(a.ui.removeMemberBtns) < len(group.Members) {
		a.ui.removeMemberBtns = append(a.ui.removeMemberBtns, widget.Clickable{})
	}

	memberWidgets := make([]layout.FlexChild, 0, len(group.Members))
	for i, member := range group.Members {
		m := member
		idx := i
		memberWidgets = append(memberWidgets, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return a.layoutMemberRow(gtx, group, m, idx)
		}))
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, memberWidgets...)
}

func (a *App) layoutMemberRow(gtx layout.Context, group *babylonapp.GroupInfo, member *babylonapp.GroupMemberInfo, index int) layout.Dimensions {
	tabletStyle := DarkClayTablet()
	tabletStyle.CornerRadius = 4
	tabletStyle.ShadowOffset = 1
	tabletStyle.Padding = layout.Inset{Left: unit.Dp(10), Right: unit.Dp(8), Top: unit.Dp(6), Bottom: unit.Dp(6)}

	return layout.Inset{Bottom: unit.Dp(3)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return ClayTablet(tabletStyle).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				// Member name
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					name := member.DisplayName
					if name == "" {
						pk := member.PubKeyHex
						if len(pk) > 12 {
							name = pk[:6] + "..." + pk[len(pk)-6:]
						} else {
							name = pk
						}
					}
					l := material.Body2(a.theme, name)
					l.Color = color.NRGBA(a.ui.theme.TextPrimary)
					l.TextSize = unit.Sp(12)
					return l.Layout(gtx)
				}),
				// Role badge
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Left: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						l := material.Caption(a.theme, member.RoleLabel)
						roleColor := color.NRGBA(a.ui.theme.TextSecondary)
						if member.Role == groups.Owner || member.Role == groups.Admin {
							roleColor = color.NRGBA(a.ui.theme.Accent)
						}
						l.Color = roleColor
						l.TextSize = unit.Sp(9)
						return l.Layout(gtx)
					})
				}),
				// Remove button (admin/owner only, can't remove self)
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if !group.IsAdmin || member.Role == groups.Owner {
						return layout.Dimensions{}
					}
					return layout.Inset{Left: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return material.Clickable(gtx, &a.ui.removeMemberBtns[index], func(gtx layout.Context) layout.Dimensions {
							l := material.Caption(a.theme, "\u2715") // ✕
							l.Color = color.NRGBA(a.ui.theme.Error)
							l.TextSize = unit.Sp(10)
							return l.Layout(gtx)
						})
					})
				}),
			)
		})
	})
}

func (a *App) layoutGroupActions(gtx layout.Context, group *babylonapp.GroupInfo) layout.Dimensions {
	children := make([]layout.FlexChild, 0, 3)

	// Leave group
	children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		label := "Leave Group"
		if a.ui.groupConfirmLeave {
			label = "Confirm Leave?"
		}
		btn := material.Button(a.theme, &a.ui.groupLeaveBtn, label)
		btn.Background = color.NRGBA(a.ui.theme.Surface)
		btn.Color = color.NRGBA(a.ui.theme.Error)
		btn.CornerRadius = unit.Dp(6)
		btn.Inset = layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8), Left: unit.Dp(16), Right: unit.Dp(16)}
		return btn.Layout(gtx)
	}))

	// Delete group (owner only)
	if group.IsOwner {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				label := "Delete Group"
				if a.ui.groupConfirmDelete {
					label = "Confirm Delete?"
				}
				btn := material.Button(a.theme, &a.ui.groupDeleteBtn, label)
				btn.Background = color.NRGBA(a.ui.theme.Error)
				btn.Color = color.NRGBA(a.ui.theme.TextPrimary)
				btn.CornerRadius = unit.Dp(6)
				btn.Inset = layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8), Left: unit.Dp(16), Right: unit.Dp(16)}
				return btn.Layout(gtx)
			})
		}))
	}

	return layout.Flex{Spacing: layout.SpaceSides}.Layout(gtx, children...)
}

// ── Create Group Dialog ─────────────────────────────────────────────────────

func (a *App) layoutCreateGroupDialog(gtx layout.Context) layout.Dimensions {
	// Semi-transparent overlay
	overlayColor := color.NRGBA{R: 0, G: 0, B: 0, A: 160}
	paint.FillShape(gtx.Ops, overlayColor, clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, gtx.Constraints.Max.Y)}.Op())

	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		maxW := gtx.Dp(420)
		if gtx.Constraints.Max.X < maxW {
			maxW = gtx.Constraints.Max.X - gtx.Dp(32)
		}
		gtx.Constraints.Min.X = maxW
		gtx.Constraints.Max.X = maxW

		return a.layoutDialogCard(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.UniformInset(unit.Dp(24)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
					// Title
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						l := material.H6(a.theme, "CREATE GROUP")
						l.Color = color.NRGBA(a.ui.theme.Accent)
						l.Font.Weight = font.Bold
						l.Alignment = text.Middle
						return l.Layout(gtx)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return a.drawDecorativeLine(gtx, 140)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),

					// Group name
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						l := material.Caption(a.theme, "Group Name")
						l.Color = color.NRGBA(a.ui.theme.TextSecondary)
						return l.Layout(gtx)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						inputStyle := DarkClayTablet()
						inputStyle.CornerRadius = 5
						inputStyle.ShadowOffset = 1
						inputStyle.Padding = layout.Inset{Left: unit.Dp(12), Right: unit.Dp(12), Top: unit.Dp(10), Bottom: unit.Dp(10)}
						return ClayTablet(inputStyle).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							ed := material.Editor(a.theme, &a.ui.createGroupName, "Enter group name...")
							ed.Color = color.NRGBA(a.ui.theme.TextPrimary)
							ed.HintColor = color.NRGBA(a.ui.theme.TextSecondary)
							ed.TextSize = unit.Sp(13)
							return ed.Layout(gtx)
						})
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),

					// Description
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						l := material.Caption(a.theme, "Description (optional)")
						l.Color = color.NRGBA(a.ui.theme.TextSecondary)
						return l.Layout(gtx)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						inputStyle := DarkClayTablet()
						inputStyle.CornerRadius = 5
						inputStyle.ShadowOffset = 1
						inputStyle.Padding = layout.Inset{Left: unit.Dp(12), Right: unit.Dp(12), Top: unit.Dp(10), Bottom: unit.Dp(10)}
						return ClayTablet(inputStyle).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							gtx.Constraints.Min.Y = gtx.Dp(50)
							ed := material.Editor(a.theme, &a.ui.createGroupDesc, "What is this group about?")
							ed.Color = color.NRGBA(a.ui.theme.TextPrimary)
							ed.HintColor = color.NRGBA(a.ui.theme.TextSecondary)
							ed.TextSize = unit.Sp(12)
							return ed.Layout(gtx)
						})
					}),

					// Error
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if a.ui.createGroupError == "" {
							return layout.Spacer{Height: unit.Dp(8)}.Layout(gtx)
						}
						return layout.Inset{Top: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							l := material.Caption(a.theme, a.ui.createGroupError)
							l.Color = color.NRGBA(a.ui.theme.Error)
							l.Alignment = text.Middle
							return l.Layout(gtx)
						})
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),

					// Buttons
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Spacing: layout.SpaceSides}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								btn := material.Button(a.theme, &a.ui.createGroupCancel, "Cancel")
								btn.Background = color.NRGBA(a.ui.theme.Surface)
								btn.Color = color.NRGBA(a.ui.theme.TextSecondary)
								btn.CornerRadius = unit.Dp(6)
								btn.Inset = layout.Inset{Top: unit.Dp(10), Bottom: unit.Dp(10), Left: unit.Dp(24), Right: unit.Dp(24)}
								return btn.Layout(gtx)
							}),
							layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								btn := material.Button(a.theme, &a.ui.createGroupSubmit, "Create")
								btn.Background = color.NRGBA(a.ui.theme.Primary)
								btn.Color = color.NRGBA(a.ui.theme.TextPrimary)
								btn.CornerRadius = unit.Dp(6)
								btn.Inset = layout.Inset{Top: unit.Dp(10), Bottom: unit.Dp(10), Left: unit.Dp(24), Right: unit.Dp(24)}
								return btn.Layout(gtx)
							}),
						)
					}),
				)
			})
		})
	})
}

// ── Group Detail Panel (overlay) ────────────────────────────────────────────

func (a *App) layoutGroupDetailPanel(gtx layout.Context) layout.Dimensions {
	group := a.selectedGroup()
	if group == nil {
		a.ui.showGroupDetail = false
		return layout.Dimensions{}
	}

	// Semi-transparent overlay
	paint.FillShape(gtx.Ops, color.NRGBA{A: 160},
		clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, gtx.Constraints.Max.Y)}.Op())

	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		maxW := gtx.Dp(500)
		maxH := gtx.Dp(500)
		if gtx.Constraints.Max.X < maxW {
			maxW = gtx.Constraints.Max.X - gtx.Dp(32)
		}
		if gtx.Constraints.Max.Y < maxH {
			maxH = gtx.Constraints.Max.Y - gtx.Dp(32)
		}
		gtx.Constraints.Min.X = maxW
		gtx.Constraints.Max.X = maxW
		gtx.Constraints.Min.Y = maxH
		gtx.Constraints.Max.Y = maxH

		// Card background
		rr := gtx.Dp(8)
		size := image.Pt(maxW, maxH)
		cardRect := clip.RRect{Rect: image.Rect(0, 0, size.X, size.Y), NE: rr, NW: rr, SE: rr, SW: rr}
		paint.FillShape(gtx.Ops, a.ui.theme.Surface, cardRect.Op(gtx.Ops))

		borderColor := a.ui.theme.Accent
		borderColor.A = 80
		paint.FillShape(gtx.Ops, borderColor, clip.Stroke{
			Path:  cardRect.Path(gtx.Ops),
			Width: float32(gtx.Dp(1)),
		}.Op())

		return layout.UniformInset(unit.Dp(20)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				// Header with close button
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							l := material.H6(a.theme, "GROUP INFO")
							l.Color = color.NRGBA(a.ui.theme.Accent)
							l.Font.Weight = font.Bold
							return l.Layout(gtx)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return material.Clickable(gtx, &a.ui.groupDetailClose, func(gtx layout.Context) layout.Dimensions {
								l := material.Body2(a.theme, "\u2715")
								l.Color = color.NRGBA(a.ui.theme.TextSecondary)
								return l.Layout(gtx)
							})
						}),
					)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return a.drawDecorativeLine(gtx, 160)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
				// Scrollable content
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return material.List(a.theme, &a.ui.groupMemberList).Layout(gtx, 1, func(gtx layout.Context, _ int) layout.Dimensions {
						return a.layoutGroupInfoCard(gtx, group)
					})
				}),
			)
		})
	})
}

// ── Group Event Handling ────────────────────────────────────────────────────

func (a *App) handleGroupEvents(gtx layout.Context) {
	// Tab switching
	if a.ui.chatsTabBtn.Clicked(gtx) {
		a.sidebarTab = "chats"
		a.selectedGroupID = ""
	}
	if a.ui.groupsTabBtn.Clicked(gtx) {
		a.sidebarTab = "groups"
		a.selectedKey = ""
		a.refreshGroups()
	}

	// Group selection
	for i, g := range a.groupList {
		if len(a.ui.groupButtons) <= i {
			break
		}
		if a.ui.groupButtons[i].Clicked(gtx) {
			a.selectGroup(g.GroupID)
		}
	}

	// Create group button
	if a.ui.createGroupBtn.Clicked(gtx) {
		a.ui.showCreateGroup = true
		a.ui.createGroupName.SetText("")
		a.ui.createGroupDesc.SetText("")
		a.ui.createGroupError = ""
	}

	// Create group dialog
	if a.ui.createGroupSubmit.Clicked(gtx) {
		a.createGroup()
	}
	// Submit on Enter in name field
	for {
		event, ok := a.ui.createGroupName.Update(gtx)
		if !ok {
			break
		}
		if _, ok := event.(widget.SubmitEvent); ok {
			a.createGroup()
		}
	}
	if a.ui.createGroupCancel.Clicked(gtx) {
		a.ui.showCreateGroup = false
	}

	// Group detail panel close
	if a.ui.groupDetailClose.Clicked(gtx) {
		a.ui.showGroupDetail = false
	}

	// Leave group
	if a.ui.groupLeaveBtn.Clicked(gtx) && a.selectedGroupID != "" {
		if a.ui.groupConfirmLeave {
			a.leaveGroup()
		} else {
			a.ui.groupConfirmLeave = true
		}
	}

	// Delete group
	if a.ui.groupDeleteBtn.Clicked(gtx) && a.selectedGroupID != "" {
		if a.ui.groupConfirmDelete {
			a.deleteGroup()
		} else {
			a.ui.groupConfirmDelete = true
		}
	}

	// Remove member buttons
	if group := a.selectedGroup(); group != nil {
		for i, member := range group.Members {
			if len(a.ui.removeMemberBtns) <= i {
				break
			}
			if a.ui.removeMemberBtns[i].Clicked(gtx) {
				a.removeMemberFromGroup(member.PubKeyHex)
			}
		}
	}
}

// ── Group Actions ───────────────────────────────────────────────────────────

func (a *App) createGroup() {
	if a.uiGroupMgr == nil {
		a.ui.createGroupError = "Not connected yet"
		return
	}

	name := a.ui.createGroupName.Text()
	if name == "" {
		a.ui.createGroupError = "Group name is required"
		return
	}

	desc := a.ui.createGroupDesc.Text()

	groupInfo, err := a.uiGroupMgr.CreateGroup(name, desc, groups.PrivateGroup)
	if err != nil {
		a.ui.createGroupError = err.Error()
		return
	}

	a.ui.showCreateGroup = false
	a.refreshGroups()
	a.selectGroup(groupInfo.GroupID)
}

func (a *App) leaveGroup() {
	if a.uiGroupMgr == nil || a.selectedGroupID == "" {
		return
	}
	if err := a.uiGroupMgr.LeaveGroup(a.selectedGroupID); err != nil {
		logger.Warnw("failed to leave group", "error", err)
		return
	}
	a.ui.showGroupDetail = false
	a.selectedGroupID = ""
	a.refreshGroups()
}

func (a *App) deleteGroup() {
	if a.uiGroupMgr == nil || a.selectedGroupID == "" {
		return
	}
	if err := a.uiGroupMgr.DeleteGroup(a.selectedGroupID); err != nil {
		logger.Warnw("failed to delete group", "error", err)
		return
	}
	a.ui.showGroupDetail = false
	a.selectedGroupID = ""
	a.refreshGroups()
}

func (a *App) removeMemberFromGroup(memberPubKeyHex string) {
	if a.uiGroupMgr == nil || a.selectedGroupID == "" {
		return
	}
	if err := a.uiGroupMgr.RemoveMember(a.selectedGroupID, memberPubKeyHex); err != nil {
		logger.Warnw("failed to remove member", "error", err)
		return
	}
	a.refreshGroups()
}
